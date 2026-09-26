using System.Buffers;
using System.Collections.Concurrent;
using System.Text;
using System.Text.Json;
using Duster.Core;

namespace Duster.Infrastructure;

/// <summary>
/// NDJSON request/reply correlation over the engine's stdio. Framing and error
/// codes follow openspec/changes/add-windows-gui/specs/engine-protocol/spec.md.
/// A malformed or unexpected line is logged and skipped, never thrown.
/// </summary>
internal sealed class EngineChannel
{
    private sealed record Pending(TaskCompletionSource<JsonElement> Reply, Action<JsonElement>? OnEvent);

    private readonly TextWriter _input;
    private readonly Action<string> _log;
    private readonly SemaphoreSlim _writeLock = new(1, 1);
    private readonly ConcurrentDictionary<long, Pending> _pending = new();
    private long _nextId;
    private volatile bool _closed;

    public EngineChannel(TextReader output, TextWriter input, Action<string> log)
    {
        _input = input;
        _log = log;
        Completion = Task.Run(() => ReadLoopAsync(output));
    }

    /// <summary>Completes when the engine's output ends; every pending request has failed by then.</summary>
    public Task Completion { get; }

    /// <summary>
    /// Sends one request and returns its <c>result</c>. Events for it go to
    /// <paramref name="onEvent"/> on the reader thread. Canceling
    /// <paramref name="ct"/> asks the engine to stop; the call still waits for
    /// the engine's reply, which then fails with <see cref="EngineErrorKind.Canceled"/>.
    /// </summary>
    public async Task<JsonElement> RequestAsync(
        string method, Action<Utf8JsonWriter>? writeParams, Action<JsonElement>? onEvent, CancellationToken ct)
    {
        ct.ThrowIfCancellationRequested();
        var id = Interlocked.Increment(ref _nextId);
        var pending = new Pending(new(TaskCreationOptions.RunContinuationsAsynchronously), onEvent);
        _pending[id] = pending;
        if (_closed && _pending.TryRemove(id, out _))
        {
            throw Exited();
        }
        await SendAsync(id, method, writeParams).ConfigureAwait(false);
        using var registration = ct.Register(() => _ = CancelAsync(id));
        return await pending.Reply.Task.ConfigureAwait(false);
    }

    /// <summary>Closes the engine's stdin: it cancels its work, stops at a safe point, and exits.</summary>
    public async Task CloseInputAsync()
    {
        await _writeLock.WaitAsync().ConfigureAwait(false);
        try
        {
            _input.Dispose();
        }
        catch (IOException)
        {
            // The engine already exited: its end of the pipe is gone, which is what closing wanted.
        }
        finally
        {
            _writeLock.Release();
        }
    }

    private async Task CancelAsync(long target)
    {
        var id = Interlocked.Increment(ref _nextId);
        _pending[id] = new Pending(new(TaskCreationOptions.RunContinuationsAsynchronously), null);
        try
        {
            await SendAsync(id, "cancel", w => w.WriteNumber("id", target)).ConfigureAwait(false);
        }
        catch (EngineException ex)
        {
            // The target fails on its own when the engine goes away.
            _log($"cancel of request {target} not sent: {ex.Message}");
        }
    }

    private async Task SendAsync(long id, string method, Action<Utf8JsonWriter>? writeParams)
    {
        var buffer = new ArrayBufferWriter<byte>();
        using (var w = new Utf8JsonWriter(buffer))
        {
            w.WriteStartObject();
            w.WriteNumber("id", id);
            w.WriteString("method", method);
            if (writeParams is not null)
            {
                w.WriteStartObject("params");
                writeParams(w);
                w.WriteEndObject();
            }
            w.WriteEndObject();
        }
        var line = Encoding.UTF8.GetString(buffer.WrittenSpan) + "\n";

        await _writeLock.WaitAsync().ConfigureAwait(false);
        try
        {
            await _input.WriteAsync(line).ConfigureAwait(false);
            await _input.FlushAsync().ConfigureAwait(false);
        }
        catch (Exception ex) when (ex is IOException or ObjectDisposedException)
        {
            _pending.TryRemove(id, out _);
            throw Exited(ex);
        }
        finally
        {
            _writeLock.Release();
        }
    }

    private async Task ReadLoopAsync(TextReader output)
    {
        try
        {
            // ponytail: no line-length cap; the engine is our own binary and caps
            // its own output. Add one if the engine ever relays untrusted text.
            while (await output.ReadLineAsync().ConfigureAwait(false) is { } line)
            {
                Dispatch(line);
            }
        }
        catch (Exception ex) when (ex is IOException or ObjectDisposedException)
        {
            _log($"engine output closed: {ex.Message}");
        }
        finally
        {
            _closed = true;
            foreach (var id in _pending.Keys)
            {
                if (_pending.TryRemove(id, out var p))
                {
                    p.Reply.TrySetException(Exited());
                }
            }
        }
    }

    private void Dispatch(string line)
    {
        if (line.Length == 0)
        {
            return;
        }
        JsonDocument doc;
        try
        {
            doc = JsonDocument.Parse(line);
        }
        catch (JsonException)
        {
            _log($"skipped a malformed engine line ({line.Length} chars)");
            return;
        }
        using (doc)
        {
            var root = doc.RootElement;
            if (root.ValueKind != JsonValueKind.Object
                || !root.TryGetProperty("id", out var idElement)
                || !idElement.TryGetInt64(out var id))
            {
                _log("skipped an engine line without an id");
                return;
            }
            if (root.TryGetProperty("event", out var eventName) && eventName.ValueKind == JsonValueKind.String)
            {
                if (_pending.TryGetValue(id, out var target) && target.OnEvent is { } onEvent
                    && root.TryGetProperty("data", out var data))
                {
                    try
                    {
                        onEvent(data.Clone());
                    }
                    catch (Exception ex)
                    {
                        _log($"event handler for request {id} failed: {ex}");
                    }
                }
                return;
            }
            if (!_pending.TryRemove(id, out var pending))
            {
                // id 0 is the engine rejecting a line it could not parse.
                _log(id == 0 ? $"engine rejected a request line: {root.GetRawText()}" : $"reply for unknown request {id}");
                return;
            }
            if (root.TryGetProperty("error", out var error) && error.ValueKind == JsonValueKind.Object)
            {
                pending.Reply.TrySetException(ToException(error));
            }
            else if (root.TryGetProperty("result", out var result))
            {
                pending.Reply.TrySetResult(result.Clone());
            }
            else
            {
                pending.Reply.TrySetException(new EngineException(EngineErrorKind.InvalidMessage,
                    "The engine sent a reply Duster could not read."));
            }
        }
    }

    internal static EngineException ToException(JsonElement error)
    {
        var code = StringProperty(error, "code");
        var message = StringProperty(error, "message") ?? code ?? "The engine reported an error.";
        JsonElement? data = error.TryGetProperty("data", out var d) ? d.Clone() : null;
        var kind = code switch
        {
            "canceled" => EngineErrorKind.Canceled,
            "bad_request" or "unknown_method" or "busy" => EngineErrorKind.Rejected,
            // Go's os.ErrPermission and Windows' ERROR_ACCESS_DENIED texts.
            _ when message.Contains("permission denied", StringComparison.OrdinalIgnoreCase)
                || message.Contains("access is denied", StringComparison.OrdinalIgnoreCase) => EngineErrorKind.PermissionDenied,
            _ => EngineErrorKind.Failed,
        };
        return new EngineException(kind, message, code, data);
    }

    private static string? StringProperty(JsonElement e, string name) =>
        e.TryGetProperty(name, out var v) && v.ValueKind == JsonValueKind.String ? v.GetString() : null;

    private static EngineException Exited(Exception? inner = null) =>
        new(EngineErrorKind.Exited, "The Duster engine stopped unexpectedly.", inner: inner);
}
