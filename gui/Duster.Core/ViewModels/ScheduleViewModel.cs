using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

public sealed class ChoiceRow(Choice choice) : SelectableRow
{
    public Choice Choice { get; } = choice;
}

/// <summary>
/// <c>du schedule</c>: one Task Scheduler task per account that runs the
/// windowless launcher. The engine validates every value with the CLI's parsers.
/// </summary>
public sealed partial class ScheduleViewModel(IEngineClient engine, IAppHost host) : ToolViewModel(engine, host)
{
    public static string[] Cadences { get; } = ["daily", "weekly", "monthly"];

    public ObservableCollection<StatRow> State { get; } = [];
    public RowList<ChoiceRow> OptIn { get; } = [];
    public ObservableCollection<StatRow> WouldClean { get; } = [];

    [ObservableProperty]
    public partial bool Loaded { get; set; }

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(TurnOnLabel))]
    public partial bool IsOn { get; set; }

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(EveryIndex))]
    public partial string Every { get; set; } = "weekly";

    /// <summary><see cref="Every"/> as a list position, for the cadence picker.</summary>
    public int EveryIndex
    {
        get => Math.Max(0, Array.IndexOf(Cadences, Every));
        set => Every = Cadences[Math.Clamp(value, 0, Cadences.Length - 1)];
    }

    public string TurnOnLabel => IsOn ? "Save changes…" : "Turn on…";

    [ObservableProperty]
    public partial TimeSpan At { get; set; } = new(19, 0, 0);

    /// <summary>Clean early below this free space on the system drive; 0 = off.</summary>
    [ObservableProperty]
    public partial double LowSpacePercent { get; set; } = 10;

    /// <summary>The categories every scheduled clean includes (never cookies, history or anything needing administrator rights).</summary>
    [ObservableProperty]
    public partial string AlwaysCleans { get; set; } = "";

    [ObservableProperty]
    public partial string Warnings { get; set; } = "";

    private Dictionary<string, string> _names = [];

    [RelayCommand]
    private Task LoadAsync() => RunAsync(ReloadAsync);

    private async Task ReloadAsync(CancellationToken ct)
    {
        var info = await Engine.CallAsync(Calls.Schedule(), null, ct);
        _names = info.Safe.Concat(info.OptIn).ToDictionary(c => c.Id, c => c.Name);
        AlwaysCleans = string.Join(", ", info.Safe.Select(c => c.Name));
        var st = info.Status;
        IsOn = st.Enabled;
        if (st.Enabled)
        {
            Every = st.Every.Length > 0 ? st.Every : Every;
            if (TimeSpan.TryParse(st.At, out var at))
            {
                At = at;
            }
            LowSpacePercent = st.LowSpacePercent ?? 0;
        }
        OptIn.Replace(info.OptIn.Select(c => new ChoiceRow(c) { IsSelected = st.Categories.Contains(c.Id) }));
        ShowState(st);
        Loaded = true;
    }

    private void ShowState(ScheduleStatus st)
    {
        State.Clear();
        State.Add(new StatRow("Scheduled cleaning", st.Enabled ? $"On: {st.Every}, checked daily at {st.At}" : "Off"));
        if (st.Enabled)
        {
            State.Add(new StatRow("Clean early", st.LowSpacePercent is { } low ? $"below {low}% free on the system drive" : "off"));
            if (st.NextCheck is { } next)
            {
                State.Add(new StatRow("Next check", next.LocalDateTime.ToString("g")));
            }
        }
        if (st.LastCheck is { } check)
        {
            State.Add(new StatRow("Last check", $"{check.Time.LocalDateTime:g}: {check.Result}"));
        }
        if (st.LastClean is { } clean)
        {
            State.Add(new StatRow("Last clean", $"{clean.Time.LocalDateTime:g}: freed {Format.Bytes(clean.Freed)} ({clean.Reason})"));
        }
        Warnings = string.Join("\n", st.Warnings.Concat(st.Notes ?? []));
    }

    private string LowSpace => LowSpacePercent < 1 ? "off" : $"{(int)Math.Min(50, LowSpacePercent)}%";

    private EngineCall<ScheduleStatus> On(bool dryRun) =>
        Calls.ScheduleOn(Every, $"{At.Hours:00}:{At.Minutes:00}", LowSpace, OptIn.Selected.Select(c => c.Choice.Id), dryRun);

    /// <summary><c>schedule on --dry-run</c>: registers nothing, shows what a run now would clean.</summary>
    [RelayCommand]
    private Task PreviewAsync() => RunAsync(async ct =>
    {
        var st = await Engine.CallAsync(On(dryRun: true), null, ct);
        WouldClean.Clear();
        foreach (var c in st.WouldClean ?? [])
        {
            WouldClean.Add(new StatRow(_names.GetValueOrDefault(c.Id, c.Id), Format.Bytes(c.Freed)));
        }
        Status = $"Preview: a run now would free about {Format.Bytes((st.WouldClean ?? []).Sum(c => c.Freed))}. Nothing was registered.";
    });

    [RelayCommand]
    private async Task TurnOnAsync()
    {
        if (IsBusy)
        {
            return; // never ask about an action that would then be ignored
        }
        var extra = OptIn.Selected.Select(c => c.Choice.Name).ToList();
        if (!await ConfirmAsync(IsOn ? "Save the schedule?" : "Turn on scheduled cleaning?",
                $"Every {Every}, checked daily at {At.Hours:00}:{At.Minutes:00}" + (LowSpace == "off" ? "" : $" or sooner below {LowSpace} free") +
                $", Duster cleans without asking: {AlwaysCleans}{(extra.Count > 0 ? ", " + string.Join(", ", extra) : "")}. " +
                "Deleted files are gone for good. It never runs as administrator and skips runs on battery.",
                IsOn ? "Save" : "Turn on"))
        {
            return;
        }
        await RunAsync(async ct =>
        {
            var st = await Engine.CallAsync(On(dryRun: false), null, ct);
            await ReloadAsync(ct);
            Status = st.Enabled ? "Scheduled cleaning is on." : "The task was not registered.";
        });
    }

    [RelayCommand]
    private async Task TurnOffAsync()
    {
        if (IsBusy)
        {
            return; // never ask about an action that would then be ignored
        }
        if (!await ConfirmAsync("Turn off scheduled cleaning?", "Duster deletes its scheduled task. The record of past runs is kept.", "Turn off"))
        {
            return;
        }
        await RunAsync(async ct =>
        {
            var r = await Engine.CallAsync(Calls.ScheduleOff(), null, ct);
            await ReloadAsync(ct);
            Status = r.AlreadyOff ? "Scheduled cleaning was already off." : "Scheduled cleaning is off.";
        });
    }
}
