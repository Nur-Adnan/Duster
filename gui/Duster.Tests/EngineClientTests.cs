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
    public void OmittedFieldsKeepTheirDefaults()
    {
        // Go omits empty fields (omitempty); a missing string must read as "", never null.
        var r = JsonSerializer.Deserialize("""{"freed":1}""", EngineJson.Default.PurgeResult)!;
        Assert.AreEqual("", r.Notice, "PurgeResult.Notice");
        var c = JsonSerializer.Deserialize("""{"id":"temp"}""", EngineJson.Default.CleanResult.Options.GetTypeInfo(typeof(CleanCategory)));
        Assert.AreEqual("", ((CleanCategory)c!).Error, "CleanCategory.Error");
    }

    [TestMethod]
    public void ParityDtosReadTheEngineWireNames()
    {
        var purge = JsonSerializer.Deserialize("""{"freed":1,"recycled":2,"kept":3,"kept_count":1,"done":2,"failed":1,"errors":["/p: busy"],"notice":"n"}""", EngineJson.Default.PurgeResult)!;
        Assert.AreEqual((1L, 2L, 3L, 1, 2), (purge.Freed, purge.Recycled, purge.Kept, purge.KeptCount, purge.Done));
        var disks = JsonSerializer.Deserialize("""{"disks":[{"id":1,"on_disk_bytes":9,"used_known":true,"blocked":"sparse","estimate_known":true}],"admin":true,"advice":[]}""", EngineJson.Default.VirtualDiskList)!;
        Assert.AreEqual((9L, "sparse", true), (disks.Disks[0].OnDiskBytes, disks.Disks[0].Blocked, disks.Disks[0].EstimateKnown));
        var schedule = JsonSerializer.Deserialize("""{"enabled":true,"every":"weekly","at":"19:00","low_space_percent":null,"categories":[],"next_check":"2026-09-27T19:00:00+06:00","last_check":null,"last_clean":null,"warnings":[]}""", EngineJson.Default.ScheduleStatus)!;
        Assert.IsNull(schedule.LowSpacePercent);
        Assert.AreEqual(19, schedule.NextCheck!.Value.Hour);
        var apps = JsonSerializer.Deserialize("""{"apps":[{"id":1,"protected":true,"per_user":true,"name":"x","install_date":"20260101"}],"admin":false}""", EngineJson.Default.AppList)!;
        Assert.IsTrue(apps.Apps[0].Protected && apps.Apps[0].PerUser);
        var installers = JsonSerializer.Deserialize("""{"items":[{"id":1,"path":"p","name":"n","size_bytes":5,"age_days":9,"modified_time":"2026-01-01T00:00:00Z","selected":true}]}""", EngineJson.Default.InstallerScan)!;
        Assert.AreEqual((5L, 9), (installers.Items[0].SizeBytes, installers.Items[0].AgeDays));
        var bench = JsonSerializer.Deserialize("""{"scan_files_per_sec":2.5,"cpu_usage_percent":7,"json_speed_per_sec":3}""", EngineJson.Default.BenchmarkMetrics)!;
        Assert.AreEqual((2.5, 7.0), (bench.ScanFilesPerSec, bench.CpuUsagePercent));
        var folder = JsonSerializer.Deserialize("""{"id":3,"path":"/r/a","trail":[{"id":1,"path":"/r"}],"entries":[],"largest":[]}""", EngineJson.Default.AnalyzeFolder)!;
        Assert.AreEqual("/r", folder.Trail.Single().Path);
    }

    [TestMethod]
    public void DtosReadTheEngineWireNames()
    {
        const string stats = """{"HostName":"pc","CPUPercent":12.5,"RAMTotal":8,"Disks":[{"Drive":"C:","Total":100,"Free":40,"Used":60}],"TopProcesses":null}""";
        var s = JsonSerializer.Deserialize(stats, EngineJson.Default.SystemStats)!;
        Assert.AreEqual("pc", s.HostName);
        Assert.AreEqual(40UL, s.Disks![0].Free);

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
    public async Task RealEnginePurgeKeepPreviewAndRestoreRoundTrip()
    {
        // The engine inherits these: its quarantine and log go to a temp profile, and
        // the log is on (CI sets DU_NO_OPLOG) because this test checks the purge was logged.
        var profile = Directory.CreateTempSubdirectory().FullName;
        var saved = (Environment.GetEnvironmentVariable("LOCALAPPDATA"), Environment.GetEnvironmentVariable("DU_NO_OPLOG"));
        Environment.SetEnvironmentVariable("LOCALAPPDATA", profile);
        Environment.SetEnvironmentVariable("DU_NO_OPLOG", null);
        try
        {
            await using var client = RealEngine();
            await client.StartAsync(TestContext.CancellationToken);
            var work = Directory.CreateTempSubdirectory().FullName;
            var modules = Path.Combine(work, "app", "node_modules");
            Directory.CreateDirectory(Path.Combine(modules, "pkg"));
            File.WriteAllBytes(Path.Combine(modules, "pkg", "index.js"), new byte[64]);
            File.WriteAllText(Path.Combine(work, "app", "package.json"), "{}");

            var scan = await client.CallAsync(Calls.PurgeScan(work), null, TestContext.CancellationToken);
            var artifact = scan.Artifacts.Single();
            Assert.AreEqual(modules, artifact.Path);
            var kept = await client.CallAsync(Calls.Purge([artifact.Id], "keep"), null, TestContext.CancellationToken);
            Assert.AreEqual((1, 64L, 0L), (kept.KeptCount, kept.Kept, kept.Freed), "kept bytes are never freed bytes");
            Assert.IsFalse(Directory.Exists(modules));

            var session = (await client.CallAsync(Calls.RestoreList(), null, TestContext.CancellationToken)).Sessions.Single();
            var preview = await client.CallAsync(Calls.Restore(session.Id, 0, dryRun: true), null, TestContext.CancellationToken);
            Assert.AreEqual("would restore", preview.Results.Single().Status);
            Assert.IsFalse(Directory.Exists(modules), "a preview moved the folder back");
            await client.CallAsync(Calls.Restore(session.Id, 0, dryRun: false), null, TestContext.CancellationToken);
            Assert.IsTrue(File.Exists(Path.Combine(modules, "pkg", "index.js")));

            var log = await client.CallAsync(Calls.Oplog(), null, TestContext.CancellationToken);
            Assert.IsTrue(log.Entries.Any(e => e.Command == "purge"), "the purge was logged");
            var plan = await client.CallAsync(Calls.RemovePlan(), null, TestContext.CancellationToken);
            Assert.AreEqual(Path.Combine(profile, "Duster"), plan.DataDir);

            var bad = await Assert.ThrowsExactlyAsync<EngineException>(() => client.CallAsync(Calls.Purge([99], "keep"), null, TestContext.CancellationToken));
            Assert.AreEqual("bad_request", bad.Code, "an ID the engine did not list is refused");
        }
        finally
        {
            Environment.SetEnvironmentVariable("LOCALAPPDATA", saved.Item1);
            Environment.SetEnvironmentVariable("DU_NO_OPLOG", saved.Item2);
        }
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

            var scan = await client.AnalyzeAsync(root, "", false, null, TestContext.CancellationToken);
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

            var session = (await client.CallAsync(Calls.RestoreList(), null, TestContext.CancellationToken)).Sessions.Single();
            Assert.AreEqual("analyze", session.Command);
            var restored = await client.CallAsync(Calls.Restore(session.Id, 0, dryRun: false), null, TestContext.CancellationToken);
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
