using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

public sealed record AppRow(InstalledApp App)
{
    public string Detail => string.Join(" · ", new[]
    {
        App.Publisher, App.Version, App.Size > 0 ? Format.Bytes(App.Size) : "",
        App.Protected ? "system component: Duster leaves it alone" : "", App.PerUser ? "installed for you only" : "",
    }.Where(s => s.Length > 0));
}

public sealed class LeftoverRow(Leftover leftover) : SelectableRow
{
    public Leftover Leftover { get; } = leftover;
    public override long Size => Leftover.Size;
}

/// <summary>
/// <c>du uninstall</c>: runs an app's own uninstaller, then offers the folders
/// it left behind, none preselected (they are matched by name), kept for 7 days.
/// </summary>
public sealed partial class AppsViewModel : ToolViewModel
{
    private IReadOnlyList<InstalledApp> _all = [];
    private bool _admin;

    public AppsViewModel(IEngineClient engine, IAppHost host) : base(engine, host)
    {
        Leftovers.SelectionChanged += () =>
        {
            HasLeftovers = Leftovers.Count > 0;
            LeftoverText = Leftovers.Summary;
            KeepLeftoversCommand.NotifyCanExecuteChanged();
        };
    }

    public ObservableCollection<AppRow> Apps { get; } = [];
    public RowList<LeftoverRow> Leftovers { get; } = [];

    [ObservableProperty]
    public partial bool Loaded { get; set; }

    [ObservableProperty]
    public partial string Search { get; set; } = "";

    [ObservableProperty]
    [NotifyCanExecuteChangedFor(nameof(UninstallCommand))]
    public partial AppRow? SelectedApp { get; set; }

    [ObservableProperty]
    public partial string LeftoverText { get; set; } = "";

    [ObservableProperty]
    public partial bool HasLeftovers { get; set; }

    /// <summary>Which app the leftovers belong to.</summary>
    [ObservableProperty]
    public partial string LeftoverApp { get; set; } = "";

    partial void OnSearchChanged(string value) => Filter();

    [RelayCommand]
    private Task LoadAsync() => RunAsync(ReloadAsync);

    private async Task ReloadAsync(CancellationToken ct)
    {
        var list = await Engine.CallAsync(Calls.Apps(), null, ct);
        _all = list.Apps.OrderBy(a => a.Name, StringComparer.CurrentCultureIgnoreCase).ToList();
        _admin = list.Admin;
        Loaded = true;
        Filter();
        Status = Format.Plural(_all.Count, "installed app", "installed apps") + ".";
    }

    private void Filter()
    {
        var q = Search.Trim();
        Apps.Clear();
        foreach (var a in _all.Where(a => q.Length == 0 || a.Name.Contains(q, StringComparison.CurrentCultureIgnoreCase)
                                              || a.Publisher.Contains(q, StringComparison.CurrentCultureIgnoreCase)))
        {
            Apps.Add(new AppRow(a));
        }
    }

    [RelayCommand(CanExecute = nameof(CanUninstall))]
    private async Task UninstallAsync()
    {
        if (IsBusy)
        {
            return; // never ask about an action that would then be ignored
        }
        var app = SelectedApp!.App;
        if (app.PerUser && _admin)
        {
            Status = $"{app.Name} is installed for you only. Run Duster without administrator rights to uninstall it.";
            return;
        }
        if (!await ConfirmAsync($"Uninstall {app.Name}?",
                $"Duster starts {app.Name}'s own uninstaller and waits for it to finish; follow its steps. " +
                "Afterwards you can choose leftover folders to keep for 7 days. Nothing else is removed.",
                "Uninstall"))
        {
            return;
        }
        await RunAsync(async ct =>
        {
            Status = $"Waiting for {app.Name}'s uninstaller…";
            var r = await Engine.CallAsync(Calls.Uninstall(app.Id), null, ct);
            Leftovers.Replace(r.Leftovers.Select(l => new LeftoverRow(l)));
            LeftoverApp = r.StillInstalled ? "" : app.Name;
            await ReloadAsync(ct);
            Status = r.StillInstalled
                ? $"{app.Name} is still installed: its uninstaller was canceled, failed quietly, or needs a restart. Nothing else was changed."
                : r.Leftovers.Count == 0
                    ? $"{app.Name} was uninstalled. No leftover folders found."
                    : $"{app.Name} was uninstalled. {Format.Plural(r.Leftovers.Count, "leftover folder", "leftover folders")} found: they are matched by name, so none are selected. Tick the ones that belong to it.";
        });
    }

    private bool CanUninstall() => SelectedApp is { App.Protected: false };

    [RelayCommand(CanExecute = nameof(CanKeepLeftovers))]
    private async Task KeepLeftoversAsync()
    {
        if (IsBusy)
        {
            return; // never ask about an action that would then be ignored
        }
        var chosen = Leftovers.Selected;
        if (!await ConfirmAsync("Remove leftover folders?",
                $"Duster will move {Format.Plural(chosen.Count, "folder", "folders")} ({Format.Bytes(chosen.Sum(r => r.Size))}) left by {LeftoverApp} into its quarantine. Restore brings them back for 7 days.",
                "Move"))
        {
            return;
        }
        await RunAsync(async ct =>
        {
            var r = await Engine.CallAsync(Calls.KeepLeftovers(chosen.Select(c => c.Leftover.Id)), null, ct);
            foreach (var row in chosen)
            {
                Leftovers.Remove(row);
            }
            Status = $"Kept {Format.Bytes(r.Bytes)} for 7 days; Restore brings it back."
                     + (r.Failed > 0 ? $" {r.Failed} could not be moved (in use or access denied) and stayed where they were." : "")
                     + (r.Notice.Length > 0 ? " " + r.Notice : "");
        });
    }

    private bool CanKeepLeftovers() => Leftovers.Selected.Count > 0;
}
