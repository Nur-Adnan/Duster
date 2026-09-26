using System.Diagnostics;
using System.Text.Json;
using Duster.Core;
using Duster.Infrastructure;

namespace Duster.Tests;

[TestClass]
public sealed class EngineClientTests
{
    public TestContext TestContext { get; set; } = null!;

    [TestMethod]
    public void ResolveRefusesMissingDirectoriesAndLinks()
    {
        var dir = Directory.CreateTempSubdirectory().FullName;
        Assert.AreEqual(EngineErrorKind.NotFound, Assert.ThrowsExactly<EngineException>(() => EnginePath.Resolve(dir)).Kind);

        Directory.CreateDirectory(Path.Combine(dir, EnginePath.FileName));
        Assert.AreEqual(EngineErrorKind.NotFound, Assert.ThrowsExactly<EngineException>(() => EnginePath.Resolve(dir)).Kind);

        var real = Path.Combine(dir, "real.exe");
        File.WriteAllText(real, "");
        Assert.AreEqual(real, EnginePath.Resolve(dir, "real.exe"));

        try
        {
            File.CreateSymbolicLink(Path.Combine(dir, "link.exe"), real);
        }
        catch (Exception ex) when (ex is IOException or UnauthorizedAccessException)
        {
            Assert.Inconclusive($"symbolic links unavailable: {ex.Message}");
        }
        Assert.AreEqual(EngineErrorKind.StartFailed, Assert.ThrowsExactly<EngineException>(() => EnginePath.Resolve(dir, "link.exe")).Kind);
    }

    [TestMethod]
    public void DtosReadTheEngineWireNames()
    {
        const string stats = """{"HostName":"pc","CPUPercent":12.5,"RAMTotal":8,"Disks":[{"Drive":"C:","Total":100,"Free":40,"Used":60}],"TopProcesses":null}""";
        var s = JsonSerializer.Deserialize(stats, EngineJson.Default.SystemStats)!;
        Assert.AreEqual("pc", s.HostName);
        Assert.AreEqual(40UL, s.Disks[0].Free);

        const string clean = """{"categories":[{"id":"prefetch","group":"System Core","bytes":5,"admin_required":true,"error":"admin_required"}],"bytes":5,"files":1}""";
        var c = JsonSerializer.Deserialize(clean, EngineJson.Default.CleanResult)!;
        Assert.IsTrue(c.Categories[0].AdminRequired);
        Assert.AreEqual("System Core", c.Categories[0].Group);
    }

    [TestMethod]
    public async Task StartFailsWithNotFoundWhenTheEngineIsMissing()
    {
        await using var client = new EngineClient(Directory.CreateTempSubdirectory().FullName, EnginePath.FileName, _ => { });
        var ex = await Assert.ThrowsExactlyAsync<EngineException>(() => client.StartAsync());
        Assert.AreEqual(EngineErrorKind.NotFound, ex.Kind);
        Assert.AreEqual(EngineState.Faulted, client.State);
        Assert.AreSame(ex, client.Fault);
    }

    [TestMethod]
    public async Task StartFailsTheHandshakeForAProgramThatIsNotTheEngine()
    {
        if (OperatingSystem.IsWindows())
        {
            Assert.Inconclusive("uses /bin/cat, which echoes the hello request back");
        }
        await using var client = new EngineClient("/bin", "cat", _ => { });
        var ex = await Assert.ThrowsExactlyAsync<EngineException>(() => client.StartAsync());
        Assert.AreEqual(EngineErrorKind.HandshakeFailed, ex.Kind);
        Assert.IsNull(client.ProcessId, "the impostor process was left running");
    }

    [TestMethod]
    public async Task RealEngineConnectsScansAndShutsDownWithoutOrphans()
    {
        var client = RealEngine();
        await client.StartAsync(TestContext.CancellationToken);
        Assert.AreEqual(EngineState.Connected, client.State);
        Assert.AreEqual(EngineClient.SupportedProtocol, client.Hello!.Protocol);
        var pid = client.ProcessId!.Value;

        var progress = new CountingProgress();
        var scan = await client.ScanCleanAsync(progress, TestContext.CancellationToken);
        Assert.IsNotEmpty(scan.Categories);
        Assert.AreEqual(scan.Categories.Count * 2, progress.Count, "one start and one done event per category");

        var doctor = await client.RunDoctorAsync(TestContext.CancellationToken);
        Assert.IsNotEmpty(doctor.Results);

        await client.DisposeAsync();
        Assert.AreEqual(EngineState.Stopped, client.State);
        AssertExited(pid);
    }

    [TestMethod]
    public async Task RealEngineCrashFaultsTheClientAndFailsRequests()
    {
        await using var client = RealEngine();
        await client.StartAsync(TestContext.CancellationToken);
        var faulted = new TaskCompletionSource();
        client.StateChanged += (_, _) =>
        {
            if (client.State == EngineState.Faulted)
            {
                faulted.TrySetResult();
            }
        };

        using (var engine = Process.GetProcessById(client.ProcessId!.Value))
        {
            engine.Kill();
        }
        await faulted.Task.WaitAsync(TimeSpan.FromSeconds(10), TestContext.CancellationToken);
        Assert.AreEqual(EngineErrorKind.Exited, client.Fault!.Kind);
        var ex = await Assert.ThrowsExactlyAsync<EngineException>(() => client.GetStatusAsync(TestContext.CancellationToken));
        Assert.AreEqual(EngineErrorKind.Exited, ex.Kind);
    }

    [TestMethod]
    public async Task RealEngineAnalyzeRecycleAndRestoreRoundTrip()
    {
        // The engine inherits these: its quarantine, history and log go to a temp profile.
        var profile = Directory.CreateTempSubdirectory().FullName;
        var saved = (Environment.GetEnvironmentVariable("LOCALAPPDATA"), Environment.GetEnvironmentVariable("DU_NO_OPLOG"));
        Environment.SetEnvironmentVariable("LOCALAPPDATA", profile);
        Environment.SetEnvironmentVariable("DU_NO_OPLOG", "1");
        try
        {
            await using var client = RealEngine();
            await client.StartAsync(TestContext.CancellationToken);
            var root = Directory.CreateTempSubdirectory().FullName;
            Directory.CreateDirectory(Path.Combine(root, "sub"));
            var victim = Path.Combine(root, "sub", "big.bin");
            File.WriteAllBytes(victim, new byte[4096]);
            File.WriteAllBytes(Path.Combine(root, "small.bin"), new byte[16]);

            var scan = await client.AnalyzeAsync(root, null, TestContext.CancellationToken);
            Assert.AreEqual(4112, scan.Root.Size);
            Assert.IsNull(scan.Changes, "first scan has nothing to compare with");
            var sub = await client.AnalyzeChildrenAsync(scan.Root.Entries.Single(e => e.IsDir).Id, TestContext.CancellationToken);
            var file = sub.Entries.Single();

            var recycled = await client.RecycleAsync(file.Id, TestContext.CancellationToken);
            Assert.AreEqual(4096, recycled.Bytes);
            Assert.IsFalse(File.Exists(victim));
            if (!recycled.Kept)
            {
                Assert.Inconclusive("the Recycle Bin took the file, so there is no quarantine session to restore");
            }

            var session = (await client.ListRestoreAsync(TestContext.CancellationToken)).Single();
            Assert.AreEqual("analyze", session.Command);
            var restored = await client.RestoreAsync(session.Id, 0, TestContext.CancellationToken);
            Assert.AreEqual("restored", restored.Results.Single().Status);
            Assert.IsTrue(File.Exists(victim));
        }
        finally
        {
            Environment.SetEnvironmentVariable("LOCALAPPDATA", saved.Item1);
            Environment.SetEnvironmentVariable("DU_NO_OPLOG", saved.Item2);
        }
    }

    /// <summary>The real `du engine`: set DUSTER_ENGINE to a built du binary (du.exe on Windows).</summary>
    private static EngineClient RealEngine()
    {
        var path = Environment.GetEnvironmentVariable("DUSTER_ENGINE");
        if (string.IsNullOrEmpty(path))
        {
            Assert.Inconclusive("DUSTER_ENGINE is not set");
        }
        return new EngineClient(Path.GetDirectoryName(Path.GetFullPath(path))!, Path.GetFileName(path), _ => { });
    }

    private static void AssertExited(int pid)
    {
        try
        {
            using var p = Process.GetProcessById(pid);
            Assert.IsTrue(p.HasExited, $"engine {pid} is still running");
        }
        catch (ArgumentException)
        {
            // Already gone.
        }
    }

    private sealed class CountingProgress : IProgress<CleanProgress>
    {
        private int _count;
        public int Count => Volatile.Read(ref _count);
        public void Report(CleanProgress value) => Interlocked.Increment(ref _count);
    }
}
