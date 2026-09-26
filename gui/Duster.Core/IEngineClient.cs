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

    /// <summary>Deletes the given categories (IDs from the latest scan). Canceling stops between categories.</summary>
    Task<CleanResult> RunCleanAsync(IReadOnlyCollection<string> ids, IProgress<CleanProgress>? progress, CancellationToken ct = default);
}
