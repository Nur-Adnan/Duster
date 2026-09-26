using System.Collections.ObjectModel;
using System.Text;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Duster.Core;

namespace Duster.Core.ViewModels;

public sealed record DriveRow(string Drive, string Summary, double UsedPercent);

public sealed record ProcessRow(string Name, string Pid, string Cpu, string Memory);

/// <summary>A label and value on the dashboard.</summary>
public sealed record StatRow(string Label, string Value);

/// <summary>
/// The <c>du status</c> dashboard. It loads on connect and on Refresh; Live
/// polls every <see cref="LiveInterval"/> only while Home is visible, and top
/// processes (a one-second sample) load only when asked.
/// </summary>
public sealed partial class HomeViewModel : ObservableObject
{
    private readonly IEngineClient _engine;
    private readonly IAppHost _host;
    private readonly Action<string> _navigate;
    private CancellationTokenSource? _live;
    private SystemStats? _last;

    public HomeViewModel(IEngineClient engine, IAppHost host, Action<string> navigate)
    {
        _engine = engine;
        _host = host;
        _navigate = navigate;
        var ui = SynchronizationContext.Current; // constructed on the UI thread
        engine.StateChanged += (_, _) => ui?.Post(_ => OnEngineStateChanged(), null);
        OnEngineStateChanged();
    }

    public TimeSpan LiveInterval { get; init; } = TimeSpan.FromSeconds(2);

    public ObservableCollection<DriveRow> Drives { get; } = [];
    public ObservableCollection<StatRow> Details { get; } = [];
    public ObservableCollection<ProcessRow> Processes { get; } = [];

    [ObservableProperty]
    public partial bool IsConnected { get; set; }

    [ObservableProperty]
    public partial bool IsLoading { get; set; }

    [ObservableProperty]
    public partial string LoadError { get; set; } = "";

    [ObservableProperty]
    public partial string Machine { get; set; } = "";

    [ObservableProperty]
    public partial string Cpu { get; set; } = "";

    [ObservableProperty]
    public partial string Memory { get; set; } = "";

    [ObservableProperty]
    public partial int HealthScore { get; set; }

    /// <summary>Refresh every <see cref="LiveInterval"/> while Home is visible.</summary>
    [ObservableProperty]
    public partial bool IsLive { get; set; }

    [ObservableProperty]
    public partial string ProcessesStatus { get; set; } = "";

    /// <summary>Set by the page: Live never polls a page nobody is looking at.</summary>
    public bool IsVisible
    {
        get;
        set
        {
            field = value;
            UpdateLive();
        }
    }

    partial void OnIsLiveChanged(bool value) => UpdateLive();

    [RelayCommand]
    private void Open(string page) => _navigate(page);

    [RelayCommand]
    private Task RefreshAsync() => LoadAsync();

    [RelayCommand]
    private async Task LoadProcessesAsync()
    {
        if (_engine.State != EngineState.Connected)
        {
            return;
        }
        ProcessesStatus = "Sampling for a second…";
        try
        {
            var stats = await _engine.CallAsync(Calls.Status(topProcesses: true));
            Processes.Clear();
            foreach (var p in stats.TopProcesses ?? [])
            {
                Processes.Add(new ProcessRow(p.Name, p.Pid.ToString(), $"{p.Cpu:0.0}%", $"{p.Memory:0.#}%"));
            }
            ProcessesStatus = Processes.Count == 0 ? "No process data." : "";
            Show(stats);
        }
        catch (EngineException ex)
        {
            ProcessesStatus = ex.Message;
        }
    }

    /// <summary>The dashboard as text on the clipboard (the GUI's <c>du status --json</c>).</summary>
    [RelayCommand]
    private void CopyReport()
    {
        if (_last is null)
        {
            return;
        }
        var sb = new StringBuilder($"Duster status, {DateTime.Now:g}\n{Machine}\nProcessor: {Cpu}\nMemory: {Memory}\n");
        foreach (var d in Drives)
        {
            sb.Append($"Drive {d.Drive} {d.Summary}\n");
        }
        foreach (var d in Details)
        {
            sb.Append($"{d.Label}: {d.Value}\n");
        }
        foreach (var p in Processes)
        {
            sb.Append($"Process {p.Name} (PID {p.Pid}) CPU {p.Cpu}, memory {p.Memory}\n");
        }
        _host.CopyText(sb.ToString());
    }

    private async Task LoadAsync()
    {
        if (IsLoading || _engine.State != EngineState.Connected)
        {
            return;
        }
        IsLoading = true;
        LoadError = "";
        try
        {
            Show(await _engine.GetStatusAsync());
        }
        catch (EngineException ex)
        {
            LoadError = ex.Message;
            IsLive = false;
        }
        finally
        {
            IsLoading = false;
        }
    }

    private void Show(SystemStats stats)
    {
        _last = stats;
        Machine = string.Join(" · ", new[] { stats.HostName, stats.OSVersion }.Where(s => s.Length > 0));
        var cores = stats.CPUCores ?? [];
        Cpu = $"{stats.CPUPercent:0}% · {stats.CPUModel}" + (cores.Count > 0 ? $" · {cores.Count} cores, busiest {cores.Max():0}%" : "");
        Memory = stats.RAMTotal > 0
            ? $"{Format.Bytes(stats.RAMUsed)} of {Format.Bytes(stats.RAMTotal)} in use ({stats.RAMPercent:0}%)"
            : "";
        HealthScore = stats.HealthScore;
        Drives.Clear();
        foreach (var d in (stats.Disks ?? []).Where(d => d.Total > 0))
        {
            Drives.Add(new DriveRow(d.Drive, $"{Format.Bytes(d.Free)} free of {Format.Bytes(d.Total)}",
                100.0 * (d.Total - d.Free) / d.Total));
        }
        Details.Clear();
        Details.Add(new StatRow("Health", $"{stats.HealthScore} / 100"));
        Details.Add(new StatRow("CPU temperature", stats.CPUTempC > 0 ? $"{stats.CPUTempC:0} °C" : "N/A (no sensor exposed)"));
        Details.Add(new StatRow("Disk activity", $"read {Format.Rate(stats.DiskReadSec)}, write {Format.Rate(stats.DiskWriteSec)}"));
        Details.Add(new StatRow("Network", $"down {Format.Rate(stats.NetDownSec)}, up {Format.Rate(stats.NetUpSec)}"));
        Details.Add(new StatRow("Battery", stats.BatteryStatus.Length == 0 && stats.BatteryLevel <= 0
            ? "No battery"
            : $"{stats.BatteryLevel}% · {stats.BatteryStatus}" + (stats.BatteryHealth.Length > 0 ? $" · health {stats.BatteryHealth}" : "")));
        Details.Add(new StatRow("Up for", Format.Duration(TimeSpan.FromSeconds(stats.UptimeSeconds))));
    }

    private void UpdateLive()
    {
        var run = IsLive && IsVisible && IsConnected;
        if (!run)
        {
            _live?.Cancel();
            _live = null;
            return;
        }
        if (_live is not null)
        {
            return;
        }
        _live = new CancellationTokenSource();
        _ = LiveLoopAsync(_live.Token);
    }

    private async Task LiveLoopAsync(CancellationToken ct)
    {
        try
        {
            while (!ct.IsCancellationRequested)
            {
                await Task.Delay(LiveInterval, ct);
                await LoadAsync();
            }
        }
        catch (OperationCanceledException)
        {
            // Live turned off, Home hidden, or the engine went away.
        }
    }

    private void OnEngineStateChanged()
    {
        IsConnected = _engine.State == EngineState.Connected;
        UpdateLive();
        if (IsConnected)
        {
            RefreshCommand.Execute(null);
        }
    }
}
