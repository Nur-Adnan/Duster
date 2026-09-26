using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

/// <summary>A listing row with its share of the folder being shown.</summary>
public sealed record AnalyzeRow(AnalyzeItem Item, double Percent)
{
    public string SizeText => Format.Bytes(Item.Size);
    public string Detail => Item.IsDir ? $"{Item.Items:N0} items" : "";
}

/// <summary>A "changes since the last scan" line; <see cref="Index"/> is its place in the engine's list.</summary>
public sealed record ChangeRow(ChangeEntry Entry, int Index)
{
    /// <summary>A folder to open for it exists in this scan (the TUI's Enter).</summary>
    public bool CanOpen => Entry.Id > 0;

    public string RevealName => $"Show {Entry.Path} in Explorer";

    public string Text => $"{Entry.Status} {(Entry.Delta >= 0 ? "+" : "-")}{Format.Bytes(Math.Abs(Entry.Delta))}";
}

/// <summary>
/// Where space goes. Scans are read-only; drill-down re-reads the engine's
/// in-memory tree; the only change is Move to Recycle Bin, done by the engine.
/// </summary>
public sealed partial class AnalyzeViewModel(IEngineClient engine, IAppHost host) : ObservableObject
{
    /// <summary>Breadcrumb: the scanned folder first, the shown folder last.</summary>
    public ObservableCollection<AnalyzeFolder> Trail { get; } = [];

    public ObservableCollection<AnalyzeRow> Entries { get; } = [];
    public ObservableCollection<AnalyzeRow> Largest { get; } = [];
    public ObservableCollection<ChangeRow> Changes { get; } = [];

    [ObservableProperty]
    public partial string Path { get; set; } = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);

    /// <summary>Choices for <see cref="Since"/>; any value <c>du analyze --since</c> accepts also works.</summary>
    public static IReadOnlyList<string> SinceChoices { get; } = [PreviousScan, "24h", "7d", "2w", "30d"];

    public const string PreviousScan = "Previous scan";

    /// <summary>Compare with a scan at least this old (<c>--since</c>), or the previous one.</summary>
    [ObservableProperty]
    public partial string Since { get; set; } = PreviousScan;

    /// <summary>Neither compare with nor save a snapshot (<c>--no-history</c>).</summary>
    [ObservableProperty]
    public partial bool NoHistory { get; set; }

    [ObservableProperty]
    [NotifyCanExecuteChangedFor(nameof(RecycleCommand), nameof(BrowseCommand))]
    public partial bool IsScanning { get; set; }

    [ObservableProperty]
    [NotifyCanExecuteChangedFor(nameof(RecycleCommand), nameof(ScanCommand))]
    public partial bool IsBusy { get; set; }

    [ObservableProperty]
    public partial string Status { get; set; } = "Pick a folder or drive and scan it. Scanning changes nothing.";

    [ObservableProperty]
    public partial string ChangesSummary { get; set; } = "";

    /// <summary>Set when a folder has more entries than one listing carries.</summary>
    [ObservableProperty]
    public partial string MoreText { get; set; } = "";

    [ObservableProperty]
    [NotifyCanExecuteChangedFor(nameof(RecycleCommand))]
    public partial AnalyzeRow? SelectedRow { get; set; }

    public AnalyzeFolder? Current => Trail.Count > 0 ? Trail[^1] : null;

    [RelayCommand(CanExecute = nameof(IsIdle))]
    private async Task BrowseAsync()
    {
        if (await host.PickFolderAsync() is { } picked)
        {
            Path = picked;
        }
    }

    [RelayCommand(IncludeCancelCommand = true, CanExecute = nameof(CanScan))]
    private async Task ScanAsync(CancellationToken ct)
    {
        IsScanning = true;
        Status = "Scanning…";
        var progress = new Progress<AnalyzeProgress>(p =>
            Status = $"Scanning… {p.Files:N0} files, {Format.Bytes(p.Bytes)} so far");
        try
        {
            var since = Since.Trim() is var s && !s.Equals(PreviousScan, StringComparison.OrdinalIgnoreCase) ? s : "";
            var result = await engine.AnalyzeAsync(Path.Trim(), since, NoHistory, progress, ct);
            Trail.Clear();
            Show(result.Root);
            ShowChanges(result.Changes);
            if (NoHistory)
            {
                ChangesSummary = "History was off for this scan: nothing was compared or saved.";
            }
            Status = $"{Format.Bytes(result.Root.Size)} in {result.Files:N0} files and {result.Dirs:N0} folders." +
                     (result.HistoryNotes.Count > 0 ? " " + string.Join(" ", result.HistoryNotes) : "");
        }
        catch (EngineException ex) when (ex.Kind == EngineErrorKind.Canceled)
        {
            Status = "Scan canceled.";
        }
        catch (EngineException ex)
        {
            Status = ex.Message;
        }
        finally
        {
            IsScanning = false;
        }
    }

    private bool CanScan() => !IsBusy;

    private bool IsIdle() => !IsScanning;

    /// <summary>Drills into a folder row; files are ignored.</summary>
    [RelayCommand]
    private Task OpenAsync(AnalyzeRow row) => !row.Item.IsDir || IsBusy || IsScanning
        ? Task.CompletedTask
        : RunAsync(async () => Show(await engine.AnalyzeChildrenAsync(row.Item.Id)));

    /// <summary>Goes back up to a breadcrumb entry.</summary>
    [RelayCommand]
    private Task GoToAsync(AnalyzeFolder folder)
    {
        var index = Trail.IndexOf(folder);
        if (index < 0 || IsBusy || IsScanning)
        {
            return Task.CompletedTask;
        }
        return RunAsync(async () =>
        {
            var fresh = await engine.AnalyzeChildrenAsync(folder.Id);
            while (Trail.Count > index)
            {
                Trail.RemoveAt(Trail.Count - 1);
            }
            Show(fresh);
        });
    }

    /// <summary>Opens the folder a change is in, with the breadcrumb from the scanned folder.</summary>
    [RelayCommand]
    private Task OpenChangeAsync(ChangeRow row) => !row.CanOpen || IsBusy || IsScanning
        ? Task.CompletedTask
        : RunAsync(async () =>
        {
            var folder = await engine.AnalyzeChildrenAsync(row.Entry.Id);
            Trail.Clear();
            foreach (var crumb in folder.Trail)
            {
                Trail.Add(new AnalyzeFolder { Id = crumb.Id, Path = crumb.Path });
            }
            Show(folder);
        });

    /// <summary>Shows a scanned item in Explorer (the engine opens it; the GUI starts nothing).</summary>
    [RelayCommand]
    private Task RevealAsync(AnalyzeRow row) => RunAsync(() => engine.CallAsync(Calls.Reveal(row.Item.Id)));

    /// <summary>Shows a change's nearest existing folder in Explorer (it may be gone).</summary>
    [RelayCommand]
    private Task RevealChangeAsync(ChangeRow row) => RunAsync(() => engine.CallAsync(Calls.Reveal(0, row.Index + 1)));

    [RelayCommand(CanExecute = nameof(CanRecycle))]
    private async Task RecycleAsync()
    {
        var row = SelectedRow!;
        if (!await host.ConfirmAsync("Move to the Recycle Bin?",
                $"{row.Item.Name} ({row.SizeText}) goes to the Recycle Bin. If the bin can't take it, Duster keeps it restorable for 7 days instead.",
                "Move to Recycle Bin"))
        {
            return;
        }
        await RunAsync(async () =>
        {
            var result = await engine.RecycleAsync(row.Item.Id);
            // Space kept in the quarantine is not freed, so it is never reported as freed.
            Status = result.Kept
                ? $"The Recycle Bin did not take {row.Item.Name}, so Duster kept it for 7 days; Restore puts it back."
                : $"Moved {row.Item.Name} ({Format.Bytes(result.Bytes)}) to the Recycle Bin.";
            var current = Current!;
            var fresh = await engine.AnalyzeChildrenAsync(current.Id);
            Trail.RemoveAt(Trail.Count - 1);
            Show(fresh);
        });
    }

    private bool CanRecycle() => !IsBusy && !IsScanning && SelectedRow is not null;

    private void Show(AnalyzeFolder folder)
    {
        Trail.Add(folder);
        OnPropertyChanged(nameof(Current));
        Fill(Entries, folder.Entries, folder.Size);
        Fill(Largest, folder.Largest, folder.Size);
        MoreText = folder.More > 0 ? $"{folder.More:N0} smaller items are not listed." : "";
        SelectedRow = null;
    }

    private void ShowChanges(ChangeReport? report)
    {
        Changes.Clear();
        if (report is null)
        {
            ChangesSummary = "First scan of this folder: the next scan will show what changed.";
            return;
        }
        for (var i = 0; i < report.Entries.Count; i++)
        {
            Changes.Add(new ChangeRow(report.Entries[i], i));
        }
        ChangesSummary = $"{(report.Delta >= 0 ? "Grew" : "Shrank")} by {Format.Bytes(Math.Abs(report.Delta))} since {report.Since.LocalDateTime:g}.";
    }

    private static void Fill(ObservableCollection<AnalyzeRow> rows, IReadOnlyList<AnalyzeItem> items, long total)
    {
        rows.Clear();
        foreach (var item in items)
        {
            rows.Add(new AnalyzeRow(item, total > 0 ? 100.0 * item.Size / total : 0));
        }
    }

    private async Task RunAsync(Func<Task> action)
    {
        IsBusy = true;
        try
        {
            await action();
        }
        catch (EngineException ex)
        {
            Status = ex.Message;
        }
        finally
        {
            IsBusy = false;
        }
    }
}
