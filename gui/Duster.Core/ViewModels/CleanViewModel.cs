using System.Collections.ObjectModel;
using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

/// <summary>One category row: selection plus what the last clean did to it.</summary>
public sealed partial class CleanItem(CleanCategory category) : ObservableObject
{
    public CleanCategory Category { get; } = category;

    /// <summary>Admin-only categories are shown but not selectable while unelevated.</summary>
    public bool CanSelect => !Category.AdminRequired;

    public string SizeText => Format.Bytes(Category.Bytes);

    [ObservableProperty]
    public partial bool IsSelected { get; set; }

    [ObservableProperty]
    public partial string Outcome { get; set; } = "";
}

/// <summary>Categories under one heading, in the engine's cleanGroups order.</summary>
public sealed class CleanGroup(string name, IEnumerable<CleanItem> items) : List<CleanItem>(items)
{
    public string Name { get; } = name;
}

/// <summary>
/// Scan, select, confirm, clean. Every delete happens in the engine
/// (clean.run with IDs from its own scan); this only chooses which categories.
/// </summary>
public sealed partial class CleanViewModel(IEngineClient engine, IAppHost host) : ObservableObject
{
    public ObservableCollection<CleanGroup> Groups { get; } = [];

    private IEnumerable<CleanItem> Items => Groups.SelectMany(g => g);

    [ObservableProperty]
    [NotifyCanExecuteChangedFor(nameof(CleanCommand), nameof(RestartAsAdministratorCommand))]
    public partial bool IsScanning { get; set; }

    [ObservableProperty]
    [NotifyCanExecuteChangedFor(nameof(CleanCommand), nameof(ScanCommand), nameof(RestartAsAdministratorCommand))]
    public partial bool IsCleaning { get; set; }

    [ObservableProperty]
    public partial double Progress { get; set; }

    [ObservableProperty]
    public partial string Status { get; set; } = "Scan to see how much space each category uses. Scanning changes nothing.";

    [ObservableProperty]
    [NotifyCanExecuteChangedFor(nameof(CleanCommand))]
    public partial int SelectedCount { get; set; }

    [ObservableProperty]
    public partial string SelectedSize { get; set; } = "";

    /// <summary>Some categories need administrator rights this process does not have.</summary>
    [ObservableProperty]
    public partial bool NeedsAdmin { get; set; }

    public bool IsBusy => IsScanning || IsCleaning;

    partial void OnIsScanningChanged(bool value) => OnPropertyChanged(nameof(IsBusy));

    partial void OnIsCleaningChanged(bool value) => OnPropertyChanged(nameof(IsBusy));

    [RelayCommand(IncludeCancelCommand = true, CanExecute = nameof(CanScan))]
    private async Task ScanAsync(CancellationToken ct)
    {
        IsScanning = true;
        Progress = 0;
        Status = "Scanning…";
        var progress = new Progress<CleanProgress>(p => Progress = Percent(p));
        try
        {
            var result = await engine.ScanCleanAsync(progress, ct);
            ShowScan(result);
            Status = $"{Format.Bytes(result.Bytes)} in {result.Files:N0} files across {result.Categories.Count} categories.";
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

    private bool CanScan() => !IsCleaning;

    [RelayCommand(IncludeCancelCommand = true, CanExecute = nameof(CanClean))]
    private async Task CleanAsync(CancellationToken ct)
    {
        var chosen = Items.Where(i => i.IsSelected && i.CanSelect).ToList();
        if (chosen.Count == 0)
        {
            return;
        }
        var bytes = chosen.Sum(i => i.Category.Bytes);
        var names = string.Join(", ", chosen.Take(5).Select(i => i.Category.Name)) + (chosen.Count > 5 ? $" and {chosen.Count - 5} more" : "");
        var confirmed = await host.ConfirmAsync(
            "Delete permanently?",
            $"Duster will permanently delete about {Format.Bytes(bytes)} from {Plural(chosen.Count, "category", "categories")}: {names}. " +
            "This can't be undone. Files that are in use are skipped.",
            "Delete");
        if (!confirmed)
        {
            return;
        }

        IsCleaning = true;
        Progress = 0;
        Status = "Cleaning…";
        foreach (var item in Items)
        {
            item.Outcome = "";
        }
        var byId = chosen.ToDictionary(i => i.Category.Id);
        var progress = new Progress<CleanProgress>(p =>
        {
            Progress = Percent(p);
            if (byId.TryGetValue(p.Category, out var item) && p.State == "start")
            {
                item.Outcome = "Cleaning…";
            }
        });
        try
        {
            var result = await engine.RunCleanAsync(chosen.Select(i => i.Category.Id).ToList(), progress, ct);
            var failed = 0;
            foreach (var c in result.Categories)
            {
                if (!byId.TryGetValue(c.Id, out var item))
                {
                    continue;
                }
                item.Outcome = Outcome(c);
                failed += c.Error.Length > 0 ? 1 : 0;
            }
            foreach (var item in chosen.Where(i => i.Outcome == "Cleaning…" || (result.Canceled && i.Outcome == "")))
            {
                item.Outcome = "Not cleaned";
            }
            Status = (result.Canceled ? "Canceled. " : "") +
                     $"Freed {Format.Bytes(result.Bytes)} ({result.Files:N0} files) from {Plural(result.Categories.Count, "category", "categories")}" +
                     (failed > 0 ? $"; {failed} reported problems (see each category)." : ".");
        }
        catch (EngineException ex)
        {
            Status = ex.Message;
        }
        finally
        {
            IsCleaning = false;
        }
    }

    private bool CanClean() => !IsScanning && !IsCleaning && SelectedCount > 0;

    [RelayCommand]
    private void SelectAll() => SetAll(true);

    [RelayCommand]
    private void SelectNone() => SetAll(false);

    [RelayCommand(CanExecute = nameof(CanRestart))]
    private void RestartAsAdministrator()
    {
        if (!host.RestartAsAdministrator())
        {
            Status = "Administrator restart was canceled.";
        }
    }

    private bool CanRestart() => !IsBusy;

    private void ShowScan(CleanResult result)
    {
        foreach (var item in Items)
        {
            item.PropertyChanged -= OnItemChanged;
        }
        Groups.Clear();
        foreach (var group in result.Categories.GroupBy(c => c.Group))
        {
            // Mirrors the TUI: everything starts selected except what cannot run.
            var items = group.Select(c => new CleanItem(c) { IsSelected = !c.AdminRequired }).ToList();
            items.ForEach(i => i.PropertyChanged += OnItemChanged);
            Groups.Add(new CleanGroup(group.Key, items));
        }
        NeedsAdmin = result.Categories.Any(c => c.AdminRequired);
        UpdateSelection();
    }

    private void SetAll(bool selected)
    {
        foreach (var item in Items.Where(i => i.CanSelect))
        {
            item.IsSelected = selected;
        }
    }

    private void OnItemChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(CleanItem.IsSelected))
        {
            UpdateSelection();
        }
    }

    private void UpdateSelection()
    {
        var chosen = Items.Where(i => i.IsSelected && i.CanSelect).ToList();
        SelectedCount = chosen.Count;
        SelectedSize = chosen.Count == 0 ? "Nothing selected" : $"{chosen.Count} selected · {Format.Bytes(chosen.Sum(i => i.Category.Bytes))}";
    }

    private static string Outcome(CleanCategory c) => c.Error switch
    {
        "" => $"Freed {Format.Bytes(c.Bytes)}",
        "admin_required" => "Needs administrator rights",
        _ when c.Bytes > 0 => $"Freed {Format.Bytes(c.Bytes)}; {c.Error}",
        _ => c.Error,
    };

    private static double Percent(CleanProgress p) =>
        p.State == "done" && p.Total > 0 ? 100.0 * (p.Index + 1) / p.Total : p.Total > 0 ? 100.0 * p.Index / p.Total : 0;

    private static string Plural(int n, string one, string many) => $"{n} {(n == 1 ? one : many)}";
}
