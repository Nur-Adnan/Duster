using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

/// <summary>
/// Duster's quarantine (the 7-day undo window): list, restore, empty. The engine
/// does every move and delete and never overwrites; this picks sessions and items.
/// </summary>
public sealed partial class RestoreViewModel : ObservableObject
{
    private readonly IEngineClient _engine;
    private readonly IAppHost _host;

    public RestoreViewModel(IEngineClient engine, IAppHost host)
    {
        _engine = engine;
        _host = host;
        var ui = SynchronizationContext.Current; // constructed on the UI thread
        engine.StateChanged += (_, _) => ui?.Post(_ => RefreshIfConnected(), null);
    }

    public ObservableCollection<RestoreSession> Sessions { get; } = [];

    [ObservableProperty]
    [NotifyCanExecuteChangedFor(nameof(RestoreSessionCommand), nameof(EmptySessionCommand))]
    [NotifyPropertyChangedFor(nameof(SelectedItems), nameof(SelectedTitle))]
    public partial RestoreSession? SelectedSession { get; set; }

    /// <summary>The selected session's items; empty when nothing is selected (x:Bind needs no null path).</summary>
    public IReadOnlyList<RestoreItem> SelectedItems => SelectedSession?.Items ?? [];

    public string SelectedTitle => SelectedSession is { } s
        ? $"{s.Command} · {s.Created.LocalDateTime:g} · {Format.Bytes(s.Size)}{(s.Damaged ? " · partly unreadable" : "")}"
        : "";

    [ObservableProperty]
    [NotifyCanExecuteChangedFor(nameof(RestoreSessionCommand), nameof(EmptySessionCommand), nameof(EmptyAllCommand), nameof(RefreshCommand))]
    public partial bool IsBusy { get; set; }

    [ObservableProperty]
    public partial string Status { get; set; } = "";

    [RelayCommand(CanExecute = nameof(IsIdle))]
    private async Task RefreshAsync()
    {
        if (_engine.State != EngineState.Connected)
        {
            return;
        }
        await RunAsync(LoadAsync);
    }

    [RelayCommand(CanExecute = nameof(HasSelection))]
    private Task RestoreSessionAsync() => RestoreAsync(SelectedSession!, 0);

    /// <summary>Restores one item of the selected session (1-based <see cref="RestoreItem.Number"/>).</summary>
    [RelayCommand]
    private Task RestoreItemAsync(RestoreItem item) =>
        SelectedSession is { } session && !IsBusy ? RestoreAsync(session, item.Number) : Task.CompletedTask;

    [RelayCommand(CanExecute = nameof(HasSelection))]
    private async Task EmptySessionAsync()
    {
        var s = SelectedSession!;
        if (await _host.ConfirmAsync("Delete permanently?",
                $"The {Plural(s.Items.Count, "item", "items")} kept by {s.Command} on {s.Created.LocalDateTime:g} ({Format.Bytes(s.Size)}) " +
                "will be deleted for good. This can't be undone.", "Delete"))
        {
            await EmptyAsync([s]);
        }
    }

    [RelayCommand(CanExecute = nameof(HasSessions))]
    private async Task EmptyAllAsync()
    {
        var all = Sessions.ToList();
        if (await _host.ConfirmAsync("Empty the quarantine?",
                $"All {Plural(all.Count, "session", "sessions")} ({Format.Bytes(all.Sum(s => s.Size))}) will be deleted for good. This can't be undone.",
                "Delete all"))
        {
            await EmptyAsync(all);
        }
    }

    private bool IsIdle() => !IsBusy;

    private bool HasSelection() => !IsBusy && SelectedSession is not null;

    private bool HasSessions() => !IsBusy && Sessions.Count > 0;

    private void RefreshIfConnected()
    {
        if (_engine.State == EngineState.Connected)
        {
            RefreshCommand.Execute(null);
        }
    }

    private async Task LoadAsync()
    {
        var selected = SelectedSession?.Id;
        var sessions = await _engine.ListRestoreAsync();
        Sessions.Clear();
        foreach (var s in sessions)
        {
            Sessions.Add(s);
        }
        SelectedSession = Sessions.FirstOrDefault(s => s.Id == selected) ?? Sessions.FirstOrDefault();
        EmptyAllCommand.NotifyCanExecuteChanged();
        if (Status.Length == 0 || sessions.Count == 0)
        {
            Status = sessions.Count == 0
                ? "Nothing is kept right now. Items Duster sets aside stay restorable for 7 days."
                : $"{Plural(sessions.Count, "session", "sessions")}, {Format.Bytes(sessions.Sum(s => s.Size))} kept.";
        }
    }

    private Task RestoreAsync(RestoreSession session, int item) => RunAsync(async () =>
    {
        var result = await _engine.RestoreAsync(session.Id, item);
        Status = Summary(result);
        await LoadAsync();
    });

    private Task EmptyAsync(IReadOnlyList<RestoreSession> sessions) => RunAsync(async () =>
    {
        var result = await _engine.EmptyRestoreAsync(sessions.Select(s => s.Id).ToList());
        Status = $"Deleted {Plural(result.Emptied, "session", "sessions")} ({Format.Bytes(result.Size)}) for good.";
        await LoadAsync();
    });

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

    internal static string Summary(RestoreRunResult result)
    {
        var parts = new List<string>();
        var restored = result.Results.Where(r => r.Status == "restored").ToList();
        if (restored.Count > 0)
        {
            parts.Add($"Restored {Plural(restored.Count, "item", "items")} ({Format.Bytes(restored.Sum(r => r.Size))})");
        }
        var skipped = result.Results.Count(r => r.Status == "skipped");
        if (skipped > 0)
        {
            parts.Add($"skipped {skipped}: something newer is at the original location, so the kept copy stays here");
        }
        var failed = result.Results.Where(r => r.Status == "failed").ToList();
        if (failed.Count > 0)
        {
            parts.Add($"{failed.Count} failed: {failed[0].Reason}");
        }
        return parts.Count == 0 ? "Nothing to restore." : string.Join("; ", parts) + ".";
    }

    private static string Plural(int n, string one, string many) => $"{n} {(n == 1 ? one : many)}";
}
