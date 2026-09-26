using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

public sealed class PurgeRow(PurgeArtifact artifact) : SelectableRow
{
    public PurgeArtifact Artifact { get; } = artifact;
    public override long Size => Artifact.Size;
    public string Detail => $"{Artifact.Framework} · {Artifact.Name}";
}

/// <summary>
/// <c>du purge</c>: build artifacts in projects under a folder (a project marker
/// is required, so they start selected as in the TUI). Kept for 7 days by
/// default; Recycle Bin or permanent delete on request.
/// </summary>
public sealed partial class PurgeViewModel : ToolViewModel
{
    public const int Keep = 0, Recycle = 1, Permanent = 2;

    public PurgeViewModel(IEngineClient engine, IAppHost host) : base(engine, host)
    {
        Items.SelectionChanged += () =>
        {
            SelectedText = Items.Summary;
            PurgeCommand.NotifyCanExecuteChanged();
        };
    }

    public RowList<PurgeRow> Items { get; } = [];

    [ObservableProperty]
    public partial string Path { get; set; } = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);

    /// <summary><see cref="Keep"/>, <see cref="Recycle"/> or <see cref="Permanent"/> (<c>--safe</c>, <c>--permanent</c>).</summary>
    [ObservableProperty]
    public partial int Mode { get; set; } = Keep;

    [ObservableProperty]
    public partial string SelectedText { get; set; } = "";

    [RelayCommand]
    private async Task BrowseAsync()
    {
        if (await Host.PickFolderAsync() is { } picked)
        {
            Path = picked;
        }
    }

    [RelayCommand]
    private Task ScanAsync() => RunAsync(async ct =>
    {
        Status = "Looking for build folders in projects…";
        var progress = new Progress<ItemProgress>(p => Status = $"Found {p.Found:N0}: {p.Path}");
        var scan = await Engine.CallAsync(Calls.PurgeScan(Path.Trim()), progress, ct);
        Items.Replace(scan.Artifacts.Select(a => new PurgeRow(a) { IsSelected = a.Selected }));
        Status = scan.Artifacts.Count == 0
            ? "No build artifacts found. Only folders inside a project (package.json, Cargo.toml and similar beside them) are offered."
            : $"{Format.Plural(scan.Artifacts.Count, "folder", "folders")}, {Format.Bytes(scan.Bytes)}. Every one rebuilds with your project's build command.";
    }, "Scan stopped.");

    [RelayCommand(CanExecute = nameof(HasSelection))]
    private async Task PurgeAsync()
    {
        if (IsBusy)
        {
            return; // never ask about an action that would then be ignored
        }
        var chosen = Items.Selected;
        var what = $"{Format.Plural(chosen.Count, "folder", "folders")} ({Format.Bytes(chosen.Sum(r => r.Size))})";
        var (title, message, label, mode) = Mode switch
        {
            Permanent => ("Delete permanently?", $"Duster will delete {what} for good. They cannot be restored.", "Delete", "permanent"),
            Recycle => ("Move to the Recycle Bin?", $"Duster will send {what} to the Recycle Bin. If the bin can't take one, Duster keeps it restorable for 7 days instead.", "Move to Recycle Bin", "recycle"),
            _ => ("Keep for 7 days?", $"Duster will move {what} into its quarantine on the same drive. Restore brings them back for 7 days; the space is freed after that, or sooner if the drive runs low.", "Move", "keep"),
        };
        if (!await ConfirmAsync(title, message, label))
        {
            return;
        }
        var call = Calls.Purge(chosen.Select(r => r.Artifact.Id), mode);
        await RunAsync(async ct =>
        {
            var progress = new Progress<ItemProgress>(p => Status = $"Removing {p.Index + 1} of {p.Total}: {p.Path}");
            try
            {
                Finish(chosen, await Engine.CallAsync(call, progress, ct), stopped: false);
            }
            catch (EngineException ex) when (ex.Kind == EngineErrorKind.Canceled && call.Partial(ex) is { } partial)
            {
                Finish(chosen, partial, stopped: true);
            }
        });
    }

    private bool HasSelection() => Items.Selected.Count > 0;

    [RelayCommand]
    private void SelectAll() => Items.SelectAll(true);

    [RelayCommand]
    private void SelectNone() => Items.SelectAll(false);

    /// <summary>Removes what was done from the list and says where the bytes went.</summary>
    private void Finish(IReadOnlyList<PurgeRow> chosen, PurgeResult r, bool stopped)
    {
        foreach (var row in chosen.Take(r.Done))
        {
            var error = r.Errors.FirstOrDefault(e => e.StartsWith(row.Artifact.Path + ":", StringComparison.Ordinal));
            if (error is null)
            {
                Items.Remove(row);
            }
            else
            {
                row.Outcome = error[(row.Artifact.Path.Length + 1)..].Trim();
            }
        }
        Status = (stopped ? "Stopped. " : "") + Describe(r);
    }

    public static string Describe(PurgeResult r)
    {
        var parts = new List<string>();
        if (r.Freed > 0)
        {
            parts.Add($"Freed {Format.Bytes(r.Freed)} (deleted for good)");
        }
        if (r.Recycled > 0)
        {
            parts.Add($"moved {Format.Bytes(r.Recycled)} to the Recycle Bin (freed when the bin is emptied)");
        }
        if (r.KeptCount > 0)
        {
            parts.Add($"kept {Format.Bytes(r.Kept)} for 7 days (Restore brings it back)");
        }
        if (r.Failed > 0)
        {
            parts.Add($"{r.Failed} failed: {r.Errors.FirstOrDefault()}");
        }
        var text = parts.Count == 0 ? "Nothing was changed." : char.ToUpperInvariant(parts[0][0]) + string.Join("; ", parts)[1..] + ".";
        return r.Notice.Length > 0 ? text + " " + r.Notice : text;
    }
}
