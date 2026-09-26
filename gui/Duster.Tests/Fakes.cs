using Duster.Core;

namespace Duster.Tests;

/// <summary>Records what a ViewModel asked of the window; confirms unless told not to.</summary>
internal sealed class FakeHost : IAppHost
{
    public bool Confirm { get; set; } = true;
    public bool Elevate { get; init; } = true;
    public string LastMessage { get; private set; } = "";
    public int Confirmations { get; private set; }
    public string? Copied { get; private set; }
    public bool Exited { get; private set; }
    public bool Restarted { get; private set; }
    public bool OpenedAppsSettings { get; private set; }

    public Task<bool> ConfirmAsync(string title, string message, string confirmLabel)
    {
        Confirmations++;
        LastMessage = message;
        return Task.FromResult(Confirm);
    }

    public bool RestartAsAdministrator() => Elevate;

    public Task<string?> PickFolderAsync() => Task.FromResult<string?>(null);

    public void CopyText(string text) => Copied = text;

    public void OpenAppsSettings() => OpenedAppsSettings = true;

    public void Exit() => Exited = true;

    public void Restart() => Restarted = true;
}

/// <summary>
/// An engine that answers from <see cref="Replies"/> (by method) and records
/// every call with its JSON parameters, so tests assert exactly what was asked.
/// </summary>
internal sealed class FakeEngine : IEngineClient
{
    public bool Connected { get; init; }
    public CleanResult Scan { get; init; } = new();
    public CleanResult CleanResult { get; init; } = new();
    public AnalyzeResult Analysis { get; init; } = new();
    public Dictionary<long, AnalyzeFolder> Folders { get; } = [];
    public RecycleResult Recycled { get; init; } = new();
    public EngineHello? Hello { get; init; }

    /// <summary>Canned replies for <see cref="CallAsync{T}"/>, by method.</summary>
    public Dictionary<string, object> Replies { get; } = [];

    /// <summary>Methods that fail instead, with this exception.</summary>
    public Dictionary<string, Exception> Failures { get; } = [];

    /// <summary>Progress events a method reports before replying.</summary>
    public Dictionary<string, ItemProgress[]> Progress { get; } = [];

    public List<(string Method, string Params)> Calls { get; } = [];

    public IReadOnlyCollection<string>? CleanedIds { get; private set; }
    public IReadOnlyCollection<string>? EmptiedIds { get; private set; }
    public long? RecycledId { get; private set; }
    public (string Since, bool NoHistory)? AnalyzeOptions { get; private set; }

    public EngineState State => Connected ? EngineState.Connected : EngineState.Stopped;
    public EngineException? Fault => null;
    public event EventHandler? StateChanged { add { } remove { } }

    /// <summary>The parameters of the last call to <paramref name="method"/>, or null.</summary>
    public string? Sent(string method) => Calls.LastOrDefault(c => c.Method == method).Params;

    public bool Asked(string method) => Calls.Any(c => c.Method == method);

    public Task StartAsync(CancellationToken ct = default) => Task.CompletedTask;
    private int _statusCalls;

    /// <summary>How often status.get was asked (Live polls from a timer, so counted atomically).</summary>
    public int StatusCalls => Volatile.Read(ref _statusCalls);

    public Task<SystemStats> GetStatusAsync(CancellationToken ct = default)
    {
        Interlocked.Increment(ref _statusCalls);
        return Task.FromResult(Replies.TryGetValue("status.get", out var s) ? (SystemStats)s : new SystemStats());
    }
    public Task<DoctorSnapshot> RunDoctorAsync(CancellationToken ct = default) => Task.FromResult(new DoctorSnapshot());
    public Task<CleanResult> ScanCleanAsync(IProgress<CleanProgress>? progress, CancellationToken ct = default) => Task.FromResult(Scan);

    public Task<CleanResult> RunCleanAsync(IReadOnlyCollection<string> ids, IProgress<CleanProgress>? progress, CancellationToken ct = default)
    {
        CleanedIds = ids;
        return Task.FromResult(CleanResult);
    }

    public Task<RestoreEmptyResult> EmptyRestoreAsync(IReadOnlyCollection<string> sessionIds, CancellationToken ct = default)
    {
        EmptiedIds = sessionIds;
        return Task.FromResult(new RestoreEmptyResult { Emptied = sessionIds.Count });
    }

    public Task<AnalyzeResult> AnalyzeAsync(string path, string since, bool noHistory, IProgress<AnalyzeProgress>? progress, CancellationToken ct = default)
    {
        AnalyzeOptions = (since, noHistory);
        return Task.FromResult(Analysis);
    }

    public Task<AnalyzeFolder> AnalyzeChildrenAsync(long id, CancellationToken ct = default) => Task.FromResult(Folders[id]);

    public Task<RecycleResult> RecycleAsync(long id, CancellationToken ct = default)
    {
        RecycledId = id;
        return Task.FromResult(Recycled);
    }

    public Task<T> CallAsync<T>(EngineCall<T> call, IProgress<ItemProgress>? progress = null, CancellationToken ct = default)
    {
        Calls.Add((call.Method, call.ParamsJson()));
        if (progress is not null && Progress.TryGetValue(call.Method, out var events))
        {
            foreach (var e in events)
            {
                progress.Report(e);
            }
        }
        if (Failures.TryGetValue(call.Method, out var ex))
        {
            return Task.FromException<T>(ex);
        }
        return Replies.TryGetValue(call.Method, out var reply)
            ? Task.FromResult((T)reply)
            : throw new InvalidOperationException("FakeEngine has no reply for " + call.Method);
    }

    public ValueTask DisposeAsync() => ValueTask.CompletedTask;
}
