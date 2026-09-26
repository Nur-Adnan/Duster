using System.ComponentModel;
using System.Diagnostics;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization.Metadata;
using Duster.Core;

namespace Duster.Infrastructure;

/// <summary>
/// Runs `du engine` from the app's folder and talks to it over stdin/stdout.
/// Orphans are prevented by the engine itself: when this process closes its
/// stdin (or dies), the engine cancels, stops at a safe point, and exits.
/// </summary>
public sealed class EngineClient(string appDirectory, string fileName, Action<string> log) : IEngineClient
{
    public const int SupportedProtocol = 1;

    private static readonly TimeSpan HandshakeTimeout = TimeSpan.FromSeconds(10);

    // ponytail: fixed grace before a kill; a very long category clean could
    // outlast it. Make it wait for the busy operation if that shows up in practice.
    private static readonly TimeSpan ExitGrace = TimeSpan.FromSeconds(30);

    private readonly SemaphoreSlim _lifecycle = new(1, 1);
    private Process? _process;
    private EngineChannel? _channel;
    private volatile bool _stopping;

    public EngineState State { get; private set; }
    public EngineException? Fault { get; private set; }
    public EngineHello? Hello { get; private set; }
    public event EventHandler? StateChanged;

    /// <summary>The engine's process id while it runs (tests check for orphans).</summary>
    internal int? ProcessId { get; private set; }

    public async Task StartAsync(CancellationToken ct = default)
    {
        await _lifecycle.WaitAsync(ct).ConfigureAwait(false);
        try
        {
            await StopCoreAsync(ExitGrace).ConfigureAwait(false);
            SetState(EngineState.Starting, null, null);
            try
            {
                var hello = await LaunchAsync(ct).ConfigureAwait(false);
                SetState(EngineState.Connected, null, hello);
            }
            catch (Exception ex) when (ex is EngineException or OperationCanceledException)
            {
                await StopCoreAsync(TimeSpan.FromSeconds(2)).ConfigureAwait(false);
                var fault = ex as EngineException
                    ?? new EngineException(EngineErrorKind.Canceled, "Starting the engine was canceled.");
                SetState(EngineState.Faulted, fault, null);
                throw fault;
            }
        }
        finally
        {
            _lifecycle.Release();
        }
    }

    public async Task<SystemStats> GetStatusAsync(CancellationToken ct = default) =>
        Read(await Channel.RequestAsync("status.get", null, null, ct).ConfigureAwait(false), EngineJson.Default.SystemStats);

    public async Task<DoctorSnapshot> RunDoctorAsync(CancellationToken ct = default) =>
        Read(await Channel.RequestAsync("doctor.run", null, null, ct).ConfigureAwait(false), EngineJson.Default.DoctorSnapshot);

    public async Task<CleanResult> ScanCleanAsync(IProgress<CleanProgress>? progress, CancellationToken ct = default) =>
        Read(await Channel.RequestAsync("clean.scan", null, OnProgress(progress), ct).ConfigureAwait(false), EngineJson.Default.CleanResult);

    public async Task<CleanResult> RunCleanAsync(
        IReadOnlyCollection<string> ids, IProgress<CleanProgress>? progress, CancellationToken ct = default)
    {
        try
        {
            var result = await Channel.RequestAsync("clean.run", w => WriteStrings(w, "ids", ids), OnProgress(progress), ct)
                .ConfigureAwait(false);
            return Read(result, EngineJson.Default.CleanResult);
        }
        catch (EngineException ex) when (ex.Kind == EngineErrorKind.Canceled && ex.Partial is { } partial)
        {
            return Read(partial, EngineJson.Default.CleanResult) with { Canceled = true };
        }
    }

    public async Task<IReadOnlyList<RestoreSession>> ListRestoreAsync(CancellationToken ct = default) =>
        Read(await Channel.RequestAsync("restore.list", null, null, ct).ConfigureAwait(false), EngineJson.Default.RestoreSessionList).Sessions;

    public async Task<RestoreRunResult> RestoreAsync(string sessionId, int item, CancellationToken ct = default) =>
        Read(await Channel.RequestAsync("restore.run", w =>
        {
            w.WriteString("id", sessionId);
            w.WriteNumber("item", item);
        }, null, ct).ConfigureAwait(false), EngineJson.Default.RestoreRunResult);

    public async Task<RestoreEmptyResult> EmptyRestoreAsync(IReadOnlyCollection<string> sessionIds, CancellationToken ct = default) =>
        Read(await Channel.RequestAsync("restore.empty", w => WriteStrings(w, "ids", sessionIds), null, ct).ConfigureAwait(false),
            EngineJson.Default.RestoreEmptyResult);

    public async Task<AnalyzeResult> AnalyzeAsync(string path, IProgress<AnalyzeProgress>? progress, CancellationToken ct = default) =>
        Read(await Channel.RequestAsync("analyze.scan", w => w.WriteString("path", path),
            progress is null ? null : data => progress.Report(Read(data, EngineJson.Default.AnalyzeProgress)), ct).ConfigureAwait(false),
            EngineJson.Default.AnalyzeResult);

    public async Task<AnalyzeFolder> AnalyzeChildrenAsync(long id, CancellationToken ct = default) =>
        Read(await Channel.RequestAsync("analyze.children", w => w.WriteNumber("id", id), null, ct).ConfigureAwait(false),
            EngineJson.Default.AnalyzeFolder);

    public async Task<RecycleResult> RecycleAsync(long id, CancellationToken ct = default) =>
        Read(await Channel.RequestAsync("analyze.recycle", w => w.WriteNumber("id", id), null, ct).ConfigureAwait(false),
            EngineJson.Default.RecycleResult);

    private static void WriteStrings(Utf8JsonWriter w, string name, IEnumerable<string> values)
    {
        w.WriteStartArray(name);
        foreach (var value in values)
        {
            w.WriteStringValue(value);
        }
        w.WriteEndArray();
    }

    public async ValueTask DisposeAsync()
    {
        await _lifecycle.WaitAsync().ConfigureAwait(false);
        try
        {
            await StopCoreAsync(ExitGrace).ConfigureAwait(false);
            SetState(EngineState.Stopped, null, null);
        }
        finally
        {
            _lifecycle.Release();
        }
    }

    private EngineChannel Channel => State == EngineState.Connected && _channel is { } channel
        ? channel
        : throw Fault ?? new EngineException(EngineErrorKind.Exited, "The Duster engine is not running.");

    private async Task<EngineHello> LaunchAsync(CancellationToken ct)
    {
        var path = EnginePath.Resolve(appDirectory, fileName);
        var startInfo = new ProcessStartInfo(path)
        {
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardInput = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            StandardInputEncoding = new UTF8Encoding(false), // a BOM would corrupt the first request
            StandardOutputEncoding = Encoding.UTF8,
            StandardErrorEncoding = Encoding.UTF8,
            WorkingDirectory = Path.GetDirectoryName(path)!,
        };
        startInfo.ArgumentList.Add("engine");

        var process = new Process { StartInfo = startInfo, EnableRaisingEvents = true };
        // Drain stderr so the engine never blocks on a full pipe.
        process.ErrorDataReceived += (_, e) =>
        {
            if (!string.IsNullOrEmpty(e.Data))
            {
                log("engine: " + e.Data);
            }
        };
        process.Exited += OnProcessExited;
        try
        {
            process.Start();
        }
        catch (Win32Exception ex)
        {
            process.Dispose();
            const int errorAccessDenied = 5;
            throw new EngineException(
                ex.NativeErrorCode == errorAccessDenied ? EngineErrorKind.PermissionDenied : EngineErrorKind.StartFailed,
                $"The Duster engine could not start: {ex.Message}", inner: ex);
        }
        process.BeginErrorReadLine();
        _process = process;
        ProcessId = process.Id;
        _channel = new EngineChannel(process.StandardOutput, process.StandardInput, log);

        EngineHello hello;
        try
        {
            var reply = await _channel.RequestAsync("hello", null, null, CancellationToken.None)
                .WaitAsync(HandshakeTimeout, ct).ConfigureAwait(false);
            hello = Read(reply, EngineJson.Default.EngineHello);
        }
        catch (Exception ex) when (ex is TimeoutException or EngineException { Kind: not EngineErrorKind.Canceled })
        {
            throw new EngineException(EngineErrorKind.HandshakeFailed,
                $"{fileName} did not answer as the Duster engine. Reinstall Duster.", inner: ex);
        }
        if (hello.Protocol != SupportedProtocol)
        {
            throw new EngineException(EngineErrorKind.ProtocolMismatch,
                $"This Duster window needs engine protocol {SupportedProtocol}, but {fileName} {hello.Version} speaks {hello.Protocol}. Reinstall Duster so both match.");
        }
        return hello;
    }

    private async Task StopCoreAsync(TimeSpan grace)
    {
        if (_process is not { } process)
        {
            return;
        }
        _stopping = true;
        try
        {
            if (_channel is { } channel)
            {
                await channel.CloseInputAsync().ConfigureAwait(false);
            }
            try
            {
                await process.WaitForExitAsync().WaitAsync(grace).ConfigureAwait(false);
            }
            catch (TimeoutException)
            {
                log($"engine did not exit within {grace.TotalSeconds:0}s of stdin closing; killing it");
                try
                {
                    process.Kill(entireProcessTree: true);
                }
                catch (InvalidOperationException)
                {
                    // Exited between the timeout and the kill.
                }
                await process.WaitForExitAsync().ConfigureAwait(false);
            }
        }
        finally
        {
            process.Exited -= OnProcessExited;
            process.Dispose();
            _process = null;
            _channel = null;
            ProcessId = null;
            _stopping = false;
        }
    }

    private void OnProcessExited(object? sender, EventArgs e)
    {
        if (_stopping || !ReferenceEquals(sender, _process))
        {
            return;
        }
        var code = sender is Process p ? p.ExitCode : -1;
        log($"engine exited unexpectedly with code {code}");
        SetState(EngineState.Faulted,
            new EngineException(EngineErrorKind.Exited, $"The Duster engine stopped unexpectedly (exit code {code})."), null);
    }

    private void SetState(EngineState state, EngineException? fault, EngineHello? hello)
    {
        State = state;
        Fault = fault;
        Hello = hello;
        StateChanged?.Invoke(this, EventArgs.Empty);
    }

    private static Action<JsonElement>? OnProgress(IProgress<CleanProgress>? progress) =>
        progress is null ? null : data => progress.Report(Read(data, EngineJson.Default.CleanProgress));

    private static T Read<T>(JsonElement element, JsonTypeInfo<T> type)
    {
        try
        {
            return element.Deserialize(type)
                ?? throw new EngineException(EngineErrorKind.InvalidMessage, "The engine sent an empty reply.");
        }
        catch (JsonException ex)
        {
            throw new EngineException(EngineErrorKind.InvalidMessage, "The engine sent a reply Duster could not read.", inner: ex);
        }
    }
}
