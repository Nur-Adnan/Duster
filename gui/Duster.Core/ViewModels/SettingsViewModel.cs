using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

/// <summary>
/// About, updates (<c>du update</c>, same release source and mandatory SHA-256)
/// and Remove Duster (<c>du remove</c>). Duster has no settings file: the
/// window follows the Windows theme, and the schedule lives on its own page.
/// </summary>
public sealed partial class SettingsViewModel(IEngineClient engine, IAppHost host, string appVersion) : ToolViewModel(engine, host)
{
    public string AppVersion { get; } = appVersion;

    public string EngineVersion => Engine.Hello?.Version ?? "not connected";

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(InstallFolder), nameof(DataFolder), nameof(SetupInstalled))]
    [NotifyCanExecuteChangedFor(nameof(RemoveCommand))]
    public partial RemovePlan? Plan { get; set; }

    public string InstallFolder => Plan is { } p ? System.IO.Path.GetDirectoryName(p.Exe) ?? "" : "";
    public string DataFolder => Plan?.DataDir ?? "";
    public bool SetupInstalled => Plan?.SetupInstalled == true;

    [ObservableProperty]
    public partial string UpdateText { get; set; } = "Duster checks GitHub only when you ask.";

    [ObservableProperty]
    [NotifyCanExecuteChangedFor(nameof(InstallUpdateCommand))]
    public partial bool UpdateAvailable { get; set; }

    [ObservableProperty]
    public partial bool RestartNeeded { get; set; }

    [ObservableProperty]
    public partial string RemoveText { get; set; } = "";

    [RelayCommand]
    private Task LoadAsync() => RunAsync(async ct =>
    {
        OnPropertyChanged(nameof(EngineVersion));
        Plan = await Engine.CallAsync(Calls.RemovePlan(), null, ct);
        RemoveText = Plan.Protected
            ? "Duster is installed in a protected system folder, so it cannot remove itself."
            : Plan.SetupInstalled
                ? "Duster was installed with its setup: remove it from Windows Settings > Apps, which also removes its entry there."
                : $"Removes du.exe, duw.exe, Duster.exe, the data folder (logs and scan history), the scheduled clean"
                  + (Plan.KeptBytes > 0 ? $" and {Format.Bytes(Plan.KeptBytes)} kept for undo" : "") + ".";
    });

    [RelayCommand]
    private Task CheckAsync() => RunAsync(async ct =>
    {
        UpdateText = "Checking…";
        var u = await Engine.CallAsync(Calls.UpdateCheck(), null, ct);
        UpdateAvailable = u.Available;
        UpdateText = u.Available
            ? $"Duster {u.Latest} is available (you have {u.Current})."
            : $"Duster {u.Current} is up to date (latest release: {u.Latest}).";
    });

    [RelayCommand(CanExecute = nameof(UpdateAvailable))]
    private Task InstallUpdateAsync() => InstallAsync(reinstall: false);

    /// <summary><c>du update --force</c>: puts back a missing duw.exe or Duster.exe.</summary>
    [RelayCommand]
    private Task ReinstallAsync() => InstallAsync(reinstall: true);

    private async Task InstallAsync(bool reinstall)
    {
        if (IsBusy)
        {
            return; // never ask about an action that would then be ignored
        }
        if (!await ConfirmAsync(reinstall ? "Reinstall Duster?" : "Install the update?",
                "Duster downloads the latest release from GitHub, checks it against its published SHA-256, and replaces du.exe, duw.exe and Duster.exe. Restart Duster afterwards.",
                reinstall ? "Reinstall" : "Install"))
        {
            return;
        }
        await RunAsync(async ct =>
        {
            UpdateText = "Downloading and verifying…";
            var r = await Engine.CallAsync(Calls.UpdateInstall(reinstall), null, ct);
            UpdateAvailable = false;
            RestartNeeded = true;
            UpdateText = $"Duster {r.Version} is installed. Restart Duster to use it.";
        });
    }

    [RelayCommand]
    private void Restart() => Host.Restart();

    [RelayCommand]
    private void OpenAppsSettings() => Host.OpenAppsSettings();

    [RelayCommand(CanExecute = nameof(CanRemove))]
    private async Task RemoveAsync()
    {
        if (IsBusy)
        {
            return; // never ask about an action that would then be ignored
        }
        if (Plan is not { Protected: false } plan)
        {
            return;
        }
        if (!await ConfirmAsync("Remove Duster?",
                $"Duster deletes itself (du.exe, duw.exe, Duster.exe), its data folder {plan.DataDir} with logs and scan history, its scheduled clean"
                + (plan.KeptBytes > 0 ? $", and {Format.Bytes(plan.KeptBytes)} it kept so you could restore it" : "")
                + ". This can't be undone. Duster closes when it is done.",
                "Remove Duster"))
        {
            return;
        }
        var removed = false;
        await RunAsync(async ct =>
        {
            await Engine.CallAsync(Calls.Remove(), null, ct);
            removed = true;
        });
        if (removed)
        {
            Host.Exit();
        }
    }

    private bool CanRemove() => Plan is { Protected: false };
}
