using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

public sealed class DiskRow(VirtualDisk disk) : SelectableRow
{
    public VirtualDisk Disk { get; } = disk;
    public override bool CanSelect => Disk.Blocked.Length == 0;
    public override long Size => Disk.OnDiskBytes;

    public string Detail => Disk.Blocked.Length > 0
        ? Disk.Blocked
        : $"{Disk.Kind.ToUpperInvariant()} · takes {Format.Bytes(Disk.OnDiskBytes)} of {Format.Bytes(Disk.Bytes)}"
          + (Disk.EstimateKnown ? $" · about {Format.Bytes(Disk.Estimate)} could come back" : "");
}

/// <summary><c>du vdisk</c>: compacts WSL and Docker Desktop disks in place (administrator).</summary>
public sealed partial class VirtualDisksViewModel : ToolViewModel
{
    public VirtualDisksViewModel(IEngineClient engine, IAppHost host) : base(engine, host)
    {
        Disks.SelectionChanged += () =>
        {
            SelectedText = Disks.Summary;
            CompactCommand.NotifyCanExecuteChanged();
        };
    }

    public RowList<DiskRow> Disks { get; } = [];
    public ObservableCollection<string> Advice { get; } = [];

    [ObservableProperty]
    [NotifyCanExecuteChangedFor(nameof(CompactCommand))]
    [NotifyPropertyChangedFor(nameof(NeedsAdmin))]
    public partial bool IsAdmin { get; set; }

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(NeedsAdmin))]
    public partial bool Scanned { get; set; }

    /// <summary>Disks are listed but compacting needs a restart as administrator.</summary>
    public bool NeedsAdmin => Scanned && !IsAdmin;

    [ObservableProperty]
    public partial string SelectedText { get; set; } = "";

    [RelayCommand]
    private Task ScanAsync() => RunAsync(async ct =>
    {
        Status = "Looking for WSL and Docker Desktop disks…";
        var list = await Engine.CallAsync(Calls.VirtualDisks(), null, ct);
        IsAdmin = list.Admin;
        Disks.Replace(list.Disks.Select(d => new DiskRow(d) { IsSelected = d.Blocked.Length == 0 }));
        Advice.Clear();
        foreach (var a in list.Advice)
        {
            Advice.Add(a);
        }
        Scanned = true;
        Status = list.Disks.Count == 0 ? "No WSL or Docker Desktop virtual disks found." : Format.Plural(list.Disks.Count, "virtual disk", "virtual disks") + ".";
    });

    [RelayCommand(CanExecute = nameof(CanCompact))]
    private async Task CompactAsync()
    {
        if (IsBusy)
        {
            return; // never ask about an action that would then be ignored
        }
        var chosen = Disks.Selected;
        if (!await ConfirmAsync("Shut down WSL and compact?",
                $"Duster runs wsl --shutdown, then compacts {Format.Plural(chosen.Count, "disk", "disks")} with diskpart. Every running distribution, shell and container stops, and unsaved work in them is lost. " +
                "Quit Docker Desktop first, or its disk stays locked and is skipped. Nothing inside the disks is read or changed. A large disk can take many minutes.",
                "Shut down and compact"))
        {
            return;
        }
        var call = Calls.Compact(chosen.Select(d => d.Disk.Id));
        await RunAsync(async ct =>
        {
            var progress = new Progress<ItemProgress>(p => Status = $"Compacting {System.IO.Path.GetFileName(p.Path)} ({p.Index + 1} of {p.Total})…");
            try
            {
                Show(await Engine.CallAsync(call, progress, ct), stopped: false);
            }
            catch (EngineException ex) when (ex.Kind == EngineErrorKind.Canceled && call.Partial(ex) is { } partial)
            {
                Show(partial, stopped: true);
            }
        });
    }

    private bool CanCompact() => IsAdmin && Disks.Selected.Count > 0;

    private void Show(VirtualDiskResult r, bool stopped)
    {
        foreach (var o in r.Results)
        {
            if (Disks.FirstOrDefault(d => d.Disk.Path == o.Path) is { } row)
            {
                row.Outcome = o.Status == "compacted" ? $"Returned {Format.Bytes(o.FreedBytes)}" : $"{o.Status}: {o.Note}";
            }
        }
        Status = (stopped ? "Stopped; the disk in progress was detached, so WSL and Docker start normally. " : "")
                 + $"Returned {Format.Bytes(r.Freed)} to the drive."
                 + (r.Warning.Length > 0 ? " " + r.Warning : "");
    }
}
