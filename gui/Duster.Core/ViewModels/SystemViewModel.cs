using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

/// <summary>A startup entry; disabling is reversible, removing is not.</summary>
public sealed record StartupRow(StartupEntry Entry, bool CanChange)
{
    public string State => Entry.Enabled ? "Enabled" : "Disabled";
    public string ToggleLabel => Entry.Enabled ? "Disable" : "Enable";
    public string ToggleName => $"{ToggleLabel} {Entry.Name}";
}

/// <summary>
/// The landing menu's Startup, Security and Drivers views. Startup loads when
/// the page opens; the security audit and the driver scan (slower: the engine
/// asks Windows for every driver) run when asked.
/// </summary>
public sealed partial class SystemViewModel(IEngineClient engine, IAppHost host) : ToolViewModel(engine, host)
{
    public ObservableCollection<StartupRow> Startup { get; } = [];
    public ObservableCollection<SecurityCheck> Security { get; } = [];
    public ObservableCollection<DriverInfo> Drivers { get; } = [];

    [ObservableProperty]
    public partial bool StartupLoaded { get; set; }

    /// <summary>Machine-wide entries need administrator rights and this process has none.</summary>
    [ObservableProperty]
    public partial bool NeedsAdmin { get; set; }

    [ObservableProperty]
    public partial string SecuritySummary { get; set; } = "Checks Microsoft Defender, the firewall, UAC, Windows Update and Remote Desktop. Changes nothing.";

    [ObservableProperty]
    public partial string DriversSummary { get; set; } = "Lists installed device drivers with their version and whether they are signed. Takes a few seconds.";

    [RelayCommand]
    private Task LoadStartupAsync() => RunAsync(ReloadStartupAsync);

    private async Task ReloadStartupAsync(CancellationToken ct)
    {
        var list = await Engine.CallAsync(Calls.StartupList(), null, ct);
        Startup.Clear();
        foreach (var e in list.Entries)
        {
            Startup.Add(new StartupRow(e, list.Admin || !e.AdminRequired));
        }
        NeedsAdmin = !list.Admin && list.Entries.Any(e => e.AdminRequired);
        StartupLoaded = true;
        var disabled = list.Entries.Count(e => !e.Enabled);
        Status = list.Entries.Count == 0
            ? "No startup apps found."
            : $"{Format.Plural(list.Entries.Count, "startup app", "startup apps")}, {disabled} disabled.";
    }

    /// <summary>Enables or disables one entry (Windows' own StartupApproved switch, reversible).</summary>
    [RelayCommand]
    private async Task ToggleAsync(StartupRow row)
    {
        if (!row.CanChange)
        {
            return;
        }
        await RunAsync(async ct =>
        {
            var r = await Engine.CallAsync(Calls.StartupToggle(row.Entry.Id), null, ct);
            await ReloadStartupAsync(ct);
            Status = $"{(r.Enabled ? "Enabled" : "Disabled")} {row.Entry.Name} at startup.";
        });
    }

    /// <summary>Removes every disabled entry for good, as the landing menu's d, d does.</summary>
    [RelayCommand]
    private async Task RemoveDisabledAsync()
    {
        if (IsBusy)
        {
            return; // never ask about an action that would then be ignored
        }
        var disabled = Startup.Where(r => !r.Entry.Enabled && r.CanChange).ToList();
        if (disabled.Count == 0)
        {
            Status = "No disabled startup apps to remove. Disable one first: that step can be undone.";
            return;
        }
        var names = string.Join(", ", disabled.Take(5).Select(r => r.Entry.Name)) + (disabled.Count > 5 ? $" and {disabled.Count - 5} more" : "");
        if (!await ConfirmAsync("Remove disabled startup apps?",
                $"Duster will delete {Format.Plural(disabled.Count, "startup entry", "startup entries")}: {names}. " +
                "Registry entries are deleted and Startup-folder shortcuts are deleted for good; the apps themselves stay installed. This can't be undone.",
                "Remove"))
        {
            return;
        }
        await RunAsync(async ct =>
        {
            var r = await Engine.CallAsync(Calls.StartupRemove(disabled.Select(d => d.Entry.Id)), null, ct);
            await ReloadStartupAsync(ct);
            Status = $"Removed {Format.Plural(r.Removed, "entry", "entries")}." + (r.Errors.Count > 0 ? $" Not removed: {string.Join("; ", r.Errors)}" : "");
        });
    }

    [RelayCommand]
    private Task CheckSecurityAsync() => RunAsync(async ct =>
    {
        SecuritySummary = "Checking…";
        var r = await Engine.CallAsync(Calls.Security(), null, ct);
        Security.Clear();
        foreach (var c in r.Checks)
        {
            Security.Add(c);
        }
        SecuritySummary = $"Security score {r.Score} / 100.";
    });

    [RelayCommand]
    private Task ListDriversAsync() => RunAsync(async ct =>
    {
        DriversSummary = "Asking Windows for the installed drivers…";
        var r = await Engine.CallAsync(Calls.Drivers(), null, ct);
        Drivers.Clear();
        foreach (var d in r.Drivers.OrderBy(d => d.Signed).ThenBy(d => d.Name, StringComparer.CurrentCultureIgnoreCase))
        {
            Drivers.Add(d);
        }
        var unsigned = r.Drivers.Count(d => !d.Signed);
        DriversSummary = $"{Format.Plural(r.Drivers.Count, "driver", "drivers")}" + (unsigned > 0 ? $", {unsigned} unsigned (listed first)." : ", all signed.");
    });
}
