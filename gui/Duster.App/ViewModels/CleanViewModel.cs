using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Duster.Core;

namespace Duster.App.ViewModels;

/// <summary>
/// Milestone 2: the read-only scan, to prove progress events and cancel end to end.
/// Selection, the confirm dialog and <see cref="IEngineClient.RunCleanAsync"/> arrive in milestone 3.
/// </summary>
public sealed partial class CleanViewModel(IEngineClient engine) : ObservableObject
{
    public ObservableCollection<CleanCategory> Categories { get; } = [];

    [ObservableProperty]
    public partial bool IsScanning { get; set; }

    [ObservableProperty]
    public partial double Progress { get; set; }

    [ObservableProperty]
    public partial string Status { get; set; } = "Scan to see how much space each category uses. Scanning changes nothing.";

    [RelayCommand(IncludeCancelCommand = true)]
    private async Task ScanAsync(CancellationToken ct)
    {
        IsScanning = true;
        Progress = 0;
        Categories.Clear();
        Status = "Scanning…";
        // Created on the UI thread, so reports arrive there.
        var progress = new Progress<CleanProgress>(p =>
        {
            if (p.State == "done" && p.Total > 0)
            {
                Progress = 100.0 * (p.Index + 1) / p.Total;
            }
        });
        try
        {
            var result = await engine.ScanCleanAsync(progress, ct);
            foreach (var category in result.Categories)
            {
                Categories.Add(category);
            }
            Status = $"{Format.Bytes(result.Bytes)} in {result.Files:N0} files across {result.Categories.Count} categories. " +
                     "Cleaning from this window is not available yet; run du clean to clean.";
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
}
