using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Duster.Core;

namespace Duster.Core.ViewModels;

public sealed record DriveRow(string Drive, string Summary, double UsedPercent);

/// <summary>Machine summary from <c>status.get</c>: loaded on connect and on Refresh, never polled.</summary>
public sealed partial class HomeViewModel : ObservableObject
{
    private readonly IEngineClient _engine;
    private readonly Action<string> _navigate;

    public HomeViewModel(IEngineClient engine, Action<string> navigate)
    {
        _engine = engine;
        _navigate = navigate;
        var ui = SynchronizationContext.Current; // constructed on the UI thread
        engine.StateChanged += (_, _) => ui?.Post(_ => OnEngineStateChanged(), null);
        OnEngineStateChanged();
    }

    public ObservableCollection<DriveRow> Drives { get; } = [];

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

    [RelayCommand]
    private void Open(string page) => _navigate(page);

    [RelayCommand]
    private async Task RefreshAsync()
    {
        if (IsLoading || _engine.State != EngineState.Connected)
        {
            return;
        }
        IsLoading = true;
        LoadError = "";
        try
        {
            var stats = await _engine.GetStatusAsync();
            Machine = string.Join(" · ", new[] { stats.HostName, stats.OSVersion }.Where(s => s.Length > 0));
            Cpu = $"{stats.CPUPercent:0}% · {stats.CPUModel}";
            Memory = stats.RAMTotal > 0
                ? $"{Format.Bytes(stats.RAMUsed)} of {Format.Bytes(stats.RAMTotal)} in use"
                : "";
            Drives.Clear();
            foreach (var d in stats.Disks.Where(d => d.Total > 0))
            {
                Drives.Add(new DriveRow(d.Drive, $"{Format.Bytes(d.Free)} free of {Format.Bytes(d.Total)}",
                    100.0 * (d.Total - d.Free) / d.Total));
            }
        }
        catch (EngineException ex)
        {
            LoadError = ex.Message;
        }
        finally
        {
            IsLoading = false;
        }
    }

    private void OnEngineStateChanged()
    {
        IsConnected = _engine.State == EngineState.Connected;
        if (IsConnected)
        {
            RefreshCommand.Execute(null);
        }
    }
}
