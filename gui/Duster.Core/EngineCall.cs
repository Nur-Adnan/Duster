using System.Text.Json;
using System.Text.Json.Serialization.Metadata;

namespace Duster.Core;

/// <summary>
/// One typed engine request: the method, its parameters and the reply type.
/// Built only by <see cref="Calls"/>, so every method name and parameter shape
/// lives in one place.
/// </summary>
public sealed record EngineCall<T>(string Method, Action<Utf8JsonWriter>? Params, JsonTypeInfo<T> Result)
{
    /// <summary>What finished before a cancel (<c>error.data</c>), or null.</summary>
    public T? Partial(EngineException ex) => ex.Partial is { ValueKind: JsonValueKind.Object } p ? p.Deserialize(Result) : default;

    /// <summary>The parameters as JSON, for tests and logs.</summary>
    public string ParamsJson()
    {
        if (Params is null)
        {
            return "";
        }
        using var buffer = new MemoryStream();
        using (var w = new Utf8JsonWriter(buffer))
        {
            w.WriteStartObject();
            Params(w);
            w.WriteEndObject();
        }
        return System.Text.Encoding.UTF8.GetString(buffer.ToArray());
    }
}

/// <summary>The engine methods beyond V1's typed client methods (engine_parity.go).</summary>
public static class Calls
{
    private static EngineJson J => EngineJson.Default;

    public static EngineCall<SystemStats> Status(bool topProcesses) =>
        new("status.get", topProcesses ? w => w.WriteBoolean("top_processes", true) : null, J.SystemStats);

    public static EngineCall<DoctorSnapshot> Doctor() => new("doctor.run", null, J.DoctorSnapshot);
    public static EngineCall<VerifyReport> Verify() => new("verify.run", null, J.VerifyReport);
    public static EngineCall<BenchmarkMetrics> Benchmark() => new("benchmark.run", null, J.BenchmarkMetrics);
    public static EngineCall<SecurityReport> Security() => new("security.run", null, J.SecurityReport);
    public static EngineCall<DriverList> Drivers() => new("drivers.list", null, J.DriverList);
    public static EngineCall<OplogList> Oplog() => new("oplog.list", null, J.OplogList);

    public static EngineCall<StartupList> StartupList() => new("startup.list", null, J.StartupList);
    public static EngineCall<StartupToggleResult> StartupToggle(int id) => new("startup.toggle", w => w.WriteNumber("id", id), J.StartupToggleResult);
    public static EngineCall<StartupRemoveResult> StartupRemove(IEnumerable<int> ids) => new("startup.remove", Ids(ids), J.StartupRemoveResult);

    public static EngineCall<PurgeScan> PurgeScan(string path) => new("purge.scan", w => w.WriteString("path", path), J.PurgeScan);

    /// <summary><paramref name="mode"/>: keep (7 days), recycle, or permanent.</summary>
    public static EngineCall<PurgeResult> Purge(IEnumerable<int> ids, string mode) => new("purge.run", w =>
    {
        Ids(ids)(w);
        w.WriteString("mode", mode);
    }, J.PurgeResult);

    public static EngineCall<InstallerScan> InstallerScan(long minSizeMb) =>
        new("installer.scan", w => w.WriteNumber("min_size_mb", minSizeMb), J.InstallerScan);

    public static EngineCall<KeepResult> KeepInstallers(IEnumerable<int> ids) => new("installer.run", Ids(ids), J.KeepResult);

    public static EngineCall<AppList> Apps() => new("uninstall.list", null, J.AppList);
    public static EngineCall<UninstallResult> Uninstall(int id) => new("uninstall.run", w => w.WriteNumber("id", id), J.UninstallResult);
    public static EngineCall<KeepResult> KeepLeftovers(IEnumerable<int> ids) => new("uninstall.sweep", Ids(ids), J.KeepResult);

    public static EngineCall<OptimizeList> OptimizeList() => new("optimize.list", null, J.OptimizeList);

    public static EngineCall<OptimizeResult> Optimize(IEnumerable<string> ids, bool dryRun) => new("optimize.run", w =>
    {
        w.WriteStartArray("ids");
        foreach (var id in ids)
        {
            w.WriteStringValue(id);
        }
        w.WriteEndArray();
        w.WriteBoolean("dry_run", dryRun);
    }, J.OptimizeResult);

    public static EngineCall<VirtualDiskList> VirtualDisks() => new("vdisk.scan", null, J.VirtualDiskList);
    public static EngineCall<VirtualDiskResult> Compact(IEnumerable<int> ids) => new("vdisk.run", Ids(ids), J.VirtualDiskResult);

    public static EngineCall<ScheduleInfo> Schedule() => new("schedule.get", null, J.ScheduleInfo);

    /// <summary><c>du schedule on</c>: the engine validates every value with the CLI's parsers.</summary>
    public static EngineCall<ScheduleStatus> ScheduleOn(string every, string at, string lowSpace, IEnumerable<string> add, bool dryRun) =>
        new("schedule.set", w =>
        {
            w.WriteString("every", every);
            w.WriteString("at", at);
            w.WriteString("low_space", lowSpace);
            w.WriteStartArray("add");
            foreach (var id in add)
            {
                w.WriteStringValue(id);
            }
            w.WriteEndArray();
            w.WriteBoolean("dry_run", dryRun);
        }, J.ScheduleStatus);

    public static EngineCall<ScheduleOffResult> ScheduleOff() => new("schedule.off", null, J.ScheduleOffResult);

    public static EngineCall<UpdateInfo> UpdateCheck() => new("update.check", null, J.UpdateInfo);
    public static EngineCall<UpdateInstallResult> UpdateInstall(bool reinstall) =>
        new("update.install", w => w.WriteBoolean("force", reinstall), J.UpdateInstallResult);

    public static EngineCall<RemovePlan> RemovePlan() => new("remove.plan", null, J.RemovePlan);
    public static EngineCall<Empty> Remove() => new("remove.run", null, J.Empty);

    public static EngineCall<RestoreSessionList> RestoreList() => new("restore.list", null, J.RestoreSessionList);

    /// <summary><paramref name="item"/> 0 = the whole session; <paramref name="dryRun"/> previews without moving.</summary>
    public static EngineCall<RestoreRunResult> Restore(string sessionId, int item, bool dryRun) => new("restore.run", w =>
    {
        w.WriteString("id", sessionId);
        w.WriteNumber("item", item);
        w.WriteBoolean("dry_run", dryRun);
    }, J.RestoreRunResult);

    /// <summary>Opens an item (or, with <paramref name="change"/> > 0, a listed change) in Explorer.</summary>
    public static EngineCall<Empty> Reveal(long id, int change = 0) => new("analyze.reveal", w =>
    {
        w.WriteNumber("id", id);
        w.WriteNumber("change", change);
    }, J.Empty);

    private static Action<Utf8JsonWriter> Ids(IEnumerable<int> ids) => w =>
    {
        w.WriteStartArray("ids");
        foreach (var id in ids)
        {
            w.WriteNumberValue(id);
        }
        w.WriteEndArray();
    };
}
