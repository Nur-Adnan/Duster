using System.Text.Json;

namespace Duster.Core;

public enum EngineState { Stopped, Starting, Connected, Faulted }

public enum EngineErrorKind
{
    NotFound,
    StartFailed,
    PermissionDenied,
    HandshakeFailed,
    ProtocolMismatch,
    Exited,
    InvalidMessage,
    Rejected,
    Canceled,
    Failed,
}

/// <summary>
/// A failure talking to the engine. <see cref="Exception.Message"/> is written
/// for users; <see cref="Code"/> is the engine's error code when it replied.
/// </summary>
public sealed class EngineException(
    EngineErrorKind kind, string message, string? code = null, JsonElement? data = null, Exception? inner = null)
    : Exception(message, inner)
{
    public EngineErrorKind Kind { get; } = kind;
    public string? Code { get; } = code;

    /// <summary>What finished before a cancel (<c>error.data</c>), e.g. a partial <see cref="CleanResult"/>.</summary>
    public JsonElement? Partial { get; } = data;
}

/// <summary>
/// The GUI's only way to reach `du engine`. It exists so ViewModel tests can
/// substitute a fake; <c>EngineClient</c> in Duster.Infrastructure is the real one.
/// </summary>
public interface IEngineClient : IAsyncDisposable
{
    EngineState State { get; }
    EngineException? Fault { get; }
    EngineHello? Hello { get; }

    /// <summary>Raised on a background thread whenever <see cref="State"/> changes.</summary>
    event EventHandler? StateChanged;

    /// <summary>Starts (or restarts) the engine and completes the handshake.</summary>
    Task StartAsync(CancellationToken ct = default);

    Task<SystemStats> GetStatusAsync(CancellationToken ct = default);
    Task<DoctorSnapshot> RunDoctorAsync(CancellationToken ct = default);

    /// <summary>Read-only: sizes every clean category.</summary>
    Task<CleanResult> ScanCleanAsync(IProgress<CleanProgress>? progress, CancellationToken ct = default);

    /// <summary>
    /// Deletes the given categories (IDs from the latest scan). Canceling stops
    /// between categories and returns what finished, with <see cref="CleanResult.Canceled"/> set.
    /// </summary>
    Task<CleanResult> RunCleanAsync(IReadOnlyCollection<string> ids, IProgress<CleanProgress>? progress, CancellationToken ct = default);

    /// <summary>Quarantine sessions Duster kept (purge, uninstall leftovers, installers, analyze).</summary>
    Task<IReadOnlyList<RestoreSession>> ListRestoreAsync(CancellationToken ct = default);

    /// <summary>Puts a listed session back (<paramref name="item"/> 0 = all). Never overwrites.</summary>
    Task<RestoreRunResult> RestoreAsync(string sessionId, int item, CancellationToken ct = default);

    /// <summary>Deletes listed sessions for good.</summary>
    Task<RestoreEmptyResult> EmptyRestoreAsync(IReadOnlyCollection<string> sessionIds, CancellationToken ct = default);

    /// <summary>Read-only size scan of a folder (also records history for "changes since").</summary>
    Task<AnalyzeResult> AnalyzeAsync(string path, IProgress<AnalyzeProgress>? progress, CancellationToken ct = default);

    /// <summary>A folder from the latest scan, by its item ID.</summary>
    Task<AnalyzeFolder> AnalyzeChildrenAsync(long id, CancellationToken ct = default);

    /// <summary>Sends a scanned item to the Recycle Bin (or quarantine when the bin refuses).</summary>
    Task<RecycleResult> RecycleAsync(long id, CancellationToken ct = default);
}

/// <summary>What ViewModels need from the window: dialogs, elevation, pickers.</summary>
public interface IAppHost
{
    /// <summary>Asks before a destructive action; true only when the user picks <paramref name="confirmLabel"/>.</summary>
    Task<bool> ConfirmAsync(string title, string message, string confirmLabel);

    /// <summary>Relaunches Duster elevated (UAC) and exits this instance; false when the user declines.</summary>
    bool RestartAsAdministrator();

    /// <summary>A folder the user picked, or null.</summary>
    Task<string?> PickFolderAsync();
}
