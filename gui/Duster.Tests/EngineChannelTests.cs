using System.Text.Json;
using System.Threading.Channels;
using Duster.Core;
using Duster.Infrastructure;

namespace Duster.Tests;

[TestClass]
public sealed class EngineChannelTests
{
    [TestMethod]
    public async Task RepliesAreCorrelatedByIdAndEventsReachTheirRequest()
    {
        var fake = new FakeEngine();
        var channel = fake.Connect();
        var events = new List<string>();

        var first = channel.RequestAsync("clean.scan", null, e => events.Add(e.GetProperty("category").GetString()!), CancellationToken.None);
        var second = channel.RequestAsync("hello", null, null, CancellationToken.None);
        var firstRequest = await fake.NextRequestAsync();
        var secondRequest = await fake.NextRequestAsync();
        Assert.AreEqual("clean.scan", firstRequest.GetProperty("method").GetString());

        var firstId = firstRequest.GetProperty("id").GetInt64();
        var secondId = secondRequest.GetProperty("id").GetInt64();
        fake.Send($$$"""{"id":{{{secondId}}},"result":{"protocol":1}}""");
        fake.Send($$$"""{"id":{{{firstId}}},"event":"progress","data":{"category":"npm"}}""");
        fake.Send($$$"""{"id":{{{firstId}}},"result":{"categories":[]}}""");

        Assert.AreEqual(1, (await second).GetProperty("protocol").GetInt32());
        await first;
        CollectionAssert.AreEqual(new[] { "npm" }, events);
    }

    [TestMethod]
    public async Task MalformedLinesAreSkippedWithoutBreakingTheStream()
    {
        var fake = new FakeEngine();
        var channel = fake.Connect();
        var request = channel.RequestAsync("hello", null, null, CancellationToken.None);
        var id = (await fake.NextRequestAsync()).GetProperty("id").GetInt64();

        fake.Send("not json");
        fake.Send("[1,2,3]");
        fake.Send("""{"no":"id"}""");
        fake.Send("""{"id":999,"result":{}}""");
        fake.Send("""{"id":0,"error":{"code":"bad_request","message":"x"}}""");
        fake.Send($$$"""{"id":{{{id}}},"result":{"ok":true}}""");

        Assert.IsTrue((await request).GetProperty("ok").GetBoolean());
        Assert.HasCount(5, fake.Logged);
    }

    [TestMethod]
    [DataRow("busy", "another operation is running", EngineErrorKind.Rejected)]
    [DataRow("unknown_method", "x", EngineErrorKind.Rejected)]
    [DataRow("failed", "open C:\\x: Access is denied.", EngineErrorKind.PermissionDenied)]
    [DataRow("failed", "disk exploded", EngineErrorKind.Failed)]
    public async Task EngineErrorsMapToKinds(string code, string message, EngineErrorKind kind)
    {
        var fake = new FakeEngine();
        var channel = fake.Connect();
        var request = channel.RequestAsync("clean.run", null, null, CancellationToken.None);
        var id = (await fake.NextRequestAsync()).GetProperty("id").GetInt64();
        fake.Send(JsonSerializer.Serialize(new { id, error = new { code, message } }));

        var ex = await Assert.ThrowsExactlyAsync<EngineException>(() => request);
        Assert.AreEqual(kind, ex.Kind);
        Assert.AreEqual(code, ex.Code);
        Assert.AreEqual(message, ex.Message);
    }

    [TestMethod]
    public async Task CancelSendsCancelAndSurfacesThePartialResult()
    {
        var fake = new FakeEngine();
        var channel = fake.Connect();
        using var cts = new CancellationTokenSource();
        var request = channel.RequestAsync("clean.run", null, null, cts.Token);
        var id = (await fake.NextRequestAsync()).GetProperty("id").GetInt64();

        await cts.CancelAsync();
        var cancel = await fake.NextRequestAsync();
        Assert.AreEqual("cancel", cancel.GetProperty("method").GetString());
        Assert.AreEqual(id, cancel.GetProperty("params").GetProperty("id").GetInt64());

        fake.Send(JsonSerializer.Serialize(new { id, error = new { code = "canceled", message = "canceled", data = new { bytes = 10 } } }));
        var ex = await Assert.ThrowsExactlyAsync<EngineException>(() => request);
        Assert.AreEqual(EngineErrorKind.Canceled, ex.Kind);
        Assert.AreEqual(10, ex.Partial!.Value.GetProperty("bytes").GetInt64());
    }

    [TestMethod]
    public async Task EngineExitFailsPendingAndLaterRequests()
    {
        var fake = new FakeEngine();
        var channel = fake.Connect();
        var request = channel.RequestAsync("clean.scan", null, null, CancellationToken.None);
        await fake.NextRequestAsync();

        fake.Exit();
        Assert.AreEqual(EngineErrorKind.Exited, (await Assert.ThrowsExactlyAsync<EngineException>(() => request)).Kind);
        await channel.Completion;
        var late = await Assert.ThrowsExactlyAsync<EngineException>(() => channel.RequestAsync("hello", null, null, CancellationToken.None));
        Assert.AreEqual(EngineErrorKind.Exited, late.Kind);
    }

    /// <summary>An in-memory engine: the test reads what the channel sends and writes what it receives.</summary>
    private sealed class FakeEngine
    {
        private readonly Channel<string> _toClient = System.Threading.Channels.Channel.CreateUnbounded<string>();
        private readonly Channel<string> _fromClient = System.Threading.Channels.Channel.CreateUnbounded<string>();

        public List<string> Logged { get; } = [];

        public EngineChannel Connect() =>
            new(new LineReader(_toClient.Reader), new LineWriter(_fromClient.Writer), m => { lock (Logged) { Logged.Add(m); } });

        public void Send(string line) => _toClient.Writer.TryWrite(line);

        public void Exit() => _toClient.Writer.TryComplete();

        public async Task<JsonElement> NextRequestAsync()
        {
            using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(5));
            return JsonDocument.Parse(await _fromClient.Reader.ReadAsync(timeout.Token)).RootElement.Clone();
        }

        private sealed class LineReader(ChannelReader<string> lines) : TextReader
        {
            public override async Task<string?> ReadLineAsync() =>
                await lines.WaitToReadAsync() && lines.TryRead(out var line) ? line : null;
        }

        private sealed class LineWriter(ChannelWriter<string> lines) : TextWriter
        {
            public override System.Text.Encoding Encoding => System.Text.Encoding.UTF8;

            public override Task WriteAsync(string? value)
            {
                foreach (var line in (value ?? "").Split('\n', StringSplitOptions.RemoveEmptyEntries))
                {
                    lines.TryWrite(line);
                }
                return Task.CompletedTask;
            }

            protected override void Dispose(bool disposing)
            {
                lines.TryComplete();
                base.Dispose(disposing);
            }
        }
    }
}
