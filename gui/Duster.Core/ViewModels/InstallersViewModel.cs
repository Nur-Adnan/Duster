using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

public sealed class InstallerRow(InstallerItem item) : SelectableRow
{
    public InstallerItem Item { get; } = item;
    public override long Size => Item.SizeBytes;
    public string Detail => $"{Item.AgeDays} days old";
}

/// <summary><c>du installer</c>: old setup files in Downloads, kept for 7 days when removed.</summary>
public sealed partial class InstallersViewModel : ToolViewModel
{
    public InstallersViewModel(IEngineClient engine, IAppHost host) : base(engine, host)
    {
        Items.SelectionChanged += () =>
        {
            SelectedText = Items.Summary;
            KeepCommand.NotifyCanExecuteChanged();
        };
    }

    public RowList<InstallerRow> Items { get; } = [];

    /// <summary><c>--min-size</c> in MB (at least 1).</summary>
    [ObservableProperty]
    public partial double MinSizeMb { get; set; } = 50;

    [ObservableProperty]
    public partial string SelectedText { get; set; } = "";

    [RelayCommand]
    private Task ScanAsync() => RunAsync(ScanCoreAsync);

    private async Task ScanCoreAsync(CancellationToken ct)
    {
        var scan = await Engine.CallAsync(Calls.InstallerScan((long)Math.Max(1, MinSizeMb)), null, ct);
        Items.Replace(scan.Items.Select(i => new InstallerRow(i) { IsSelected = true }));
        Status = scan.Items.Count == 0
            ? "No installers (.exe, .msi, .msix, .appx) at least 7 days old and this big in Downloads."
            : $"{Format.Plural(scan.Items.Count, "installer", "installers")} in Downloads, {Format.Bytes(scan.Items.Sum(i => i.SizeBytes))}.";
    }

    [RelayCommand(CanExecute = nameof(HasSelection))]
    private async Task KeepAsync()
    {
        if (IsBusy)
        {
            return; // never ask about an action that would then be ignored
        }
        var chosen = Items.Selected;
        if (!await ConfirmAsync("Remove old installers?",
                $"Duster will move {Format.Plural(chosen.Count, "installer", "installers")} ({Format.Bytes(chosen.Sum(r => r.Size))}) out of Downloads into its quarantine. Restore brings them back for 7 days.",
                "Move"))
        {
            return;
        }
        await RunAsync(async ct =>
        {
            var r = await Engine.CallAsync(Calls.KeepInstallers(chosen.Select(c => c.Item.Id)), null, ct);
            await ScanCoreAsync(ct);
            Status = $"Kept {Format.Bytes(r.Bytes)} in {Format.Plural(r.Kept, "installer", "installers")} for 7 days; Restore brings them back."
                     + (r.Failed > 0 ? $" {r.Failed} could not be moved and stayed in Downloads." : "")
                     + (r.Notice.Length > 0 ? " " + r.Notice : "");
        });
    }

    private bool HasSelection() => Items.Selected.Count > 0;

    [RelayCommand]
    private void SelectAll() => Items.SelectAll(true);

    [RelayCommand]
    private void SelectNone() => Items.SelectAll(false);
}
