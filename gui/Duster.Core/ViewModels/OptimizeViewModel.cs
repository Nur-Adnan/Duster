using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

public sealed class OptimizeRow(OptimizeTask task, bool admin) : SelectableRow
{
    public OptimizeTask Task { get; } = task;

    /// <summary>Administrator-only tasks cannot be chosen while unelevated (the engine would skip them).</summary>
    public override bool CanSelect => admin || !Task.AdminRequired;
}

public sealed record ReclaimRow(ReclaimItem Item)
{
    public string SizeText => !Item.Present ? "Not present" : Format.Bytes(Item.Bytes) + (Item.Partial ? " or more" : "");
    public string Hints => string.Join("\n", Item.Hints);
}

/// <summary>
/// <c>du optimize</c>: DNS flush, Delivery Optimization cache, TRIM, and the
/// component store (<c>--deep</c>, never preselected), plus the reclaim
/// report of what only Windows may remove.
/// </summary>
public sealed partial class OptimizeViewModel : ToolViewModel
{
    public OptimizeViewModel(IEngineClient engine, IAppHost host) : base(engine, host)
    {
        Tasks.SelectionChanged += () =>
        {
            OptimizeCommand.NotifyCanExecuteChanged();
            PreviewCommand.NotifyCanExecuteChanged();
        };
    }

    public RowList<OptimizeRow> Tasks { get; } = [];
    public ObservableCollection<ReclaimRow> Reclaim { get; } = [];

    [ObservableProperty]
    public partial bool Loaded { get; set; }

    [ObservableProperty]
    public partial bool NeedsAdmin { get; set; }

    [RelayCommand]
    private Task LoadAsync() => RunAsync(async ct =>
    {
        Status = "Measuring…";
        var list = await Engine.CallAsync(Calls.OptimizeList(), null, ct);
        Tasks.Replace(list.Tasks.Select(t => new OptimizeRow(t, list.Admin)
        {
            IsSelected = t.Id != "component_store" && (list.Admin || !t.AdminRequired),
        }));
        Reclaim.Clear();
        foreach (var r in list.Reclaim)
        {
            Reclaim.Add(new ReclaimRow(r));
        }
        NeedsAdmin = !list.Admin;
        Loaded = true;
        Status = "";
    });

    /// <summary><c>--dry-run</c>: only the component store runs, as DISM's read-only analysis.</summary>
    [RelayCommand(CanExecute = nameof(HasSelection))]
    private Task PreviewAsync() => RunTasksAsync(dryRun: true);

    [RelayCommand(CanExecute = nameof(HasSelection))]
    private async Task OptimizeAsync()
    {
        if (IsBusy)
        {
            return; // never ask about an action that would then be ignored
        }
        var chosen = Tasks.Selected;
        var deep = chosen.Any(r => r.Task.Id == "component_store");
        if (!await ConfirmAsync("Optimize now?",
                $"Duster will run: {string.Join(", ", chosen.Select(r => r.Task.Name))}. " +
                "The Delivery Optimization cache is deleted for good (Windows downloads it again when needed)." +
                (deep ? " The component store cleanup can run for 10 minutes or more and cannot be undone; stopping it part-way leaves Windows to finish servicing later." : ""),
                "Optimize"))
        {
            return;
        }
        await RunTasksAsync(dryRun: false);
    }

    private bool HasSelection() => Tasks.Selected.Count > 0;

    private async Task RunTasksAsync(bool dryRun)
    {
        var chosen = Tasks.Selected;
        var call = Calls.Optimize(chosen.Select(r => r.Task.Id), dryRun);
        await RunAsync(async ct =>
        {
            foreach (var row in chosen)
            {
                row.Outcome = "";
            }
            var progress = new Progress<ItemProgress>(p =>
            {
                if (p.State == "start" && Tasks.FirstOrDefault(r => r.Task.Id == p.Task) is { } row)
                {
                    row.Outcome = "Running…";
                }
            });
            try
            {
                Show(await Engine.CallAsync(call, progress, ct), dryRun, stopped: false);
            }
            catch (EngineException ex) when (ex.Kind == EngineErrorKind.Canceled && call.Partial(ex) is { } partial)
            {
                Show(partial, dryRun, stopped: true);
            }
        });
    }

    private void Show(OptimizeResult r, bool dryRun, bool stopped)
    {
        foreach (var t in r.Tasks)
        {
            if (Tasks.FirstOrDefault(row => row.Task.Id == t.Id) is { } row)
            {
                row.Outcome = string.Join(" · ", new[]
                {
                    Capitalize(t.Status), t.Reclaimed > 0 ? $"reclaimed {Format.Bytes(t.Reclaimed)}" : "", t.Note, t.Error,
                }.Where(s => s.Length > 0));
            }
        }
        foreach (var row in Tasks.Where(row => row.Outcome == "Running…"))
        {
            row.Outcome = "Not run";
        }
        Status = (stopped ? "Stopped. " : "") + (dryRun
            ? "Preview: nothing was changed."
            : $"Done. Reclaimed {Format.Bytes(r.Reclaimed)}" + (r.Tasks.Any(t => t.Status == "failed") ? "; some tasks failed (see each one)." : "."));
    }

    private static string Capitalize(string s) => s.Length == 0 ? s : char.ToUpperInvariant(s[0]) + s[1..];
}
