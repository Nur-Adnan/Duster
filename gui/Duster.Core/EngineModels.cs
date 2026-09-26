using System.Text.Json.Serialization;

namespace Duster.Core;

// Protocol DTOs for `du engine` (cmd/engine.go). Properties use set, not init:
// the source generator assigns every init property on creation, so a field the
// engine omits (Go omitempty) would become null instead of keeping its default.
// Engine types use snake_case
// names; SystemStats mirrors lib/sysinfo, which has no json tags, so its
// properties carry the Go field names explicitly.

/// <summary>Reply to <c>hello</c>.</summary>
public sealed record EngineHello
{
    public int Protocol { get; set; }
    public string Version { get; set; } = "";
    public bool Admin { get; set; }
}

/// <summary>Reply to <c>status.get</c>: everything <c>du status --json</c> reports.</summary>
public sealed record SystemStats
{
    [JsonPropertyName("HostName")] public string HostName { get; set; } = "";
    [JsonPropertyName("OSVersion")] public string OSVersion { get; set; } = "";
    [JsonPropertyName("CPUModel")] public string CPUModel { get; set; } = "";
    [JsonPropertyName("CPUPercent")] public double CPUPercent { get; set; }
    [JsonPropertyName("CPUCores")] public IReadOnlyList<double>? CPUCores { get; set; }

    /// <summary>Hottest ACPI thermal zone in °C; 0 when the machine exposes none.</summary>
    [JsonPropertyName("CPUTempC")] public double CPUTempC { get; set; }
    [JsonPropertyName("RAMTotal")] public ulong RAMTotal { get; set; }
    [JsonPropertyName("RAMUsed")] public ulong RAMUsed { get; set; }
    [JsonPropertyName("RAMAvail")] public ulong RAMAvail { get; set; }
    [JsonPropertyName("RAMPercent")] public double RAMPercent { get; set; }
    [JsonPropertyName("Disks")] public IReadOnlyList<DiskInfo>? Disks { get; set; }
    [JsonPropertyName("DiskReadSec")] public ulong DiskReadSec { get; set; }
    [JsonPropertyName("DiskWriteSec")] public ulong DiskWriteSec { get; set; }
    [JsonPropertyName("NetDownSec")] public ulong NetDownSec { get; set; }
    [JsonPropertyName("NetUpSec")] public ulong NetUpSec { get; set; }
    [JsonPropertyName("BatteryLevel")] public int BatteryLevel { get; set; }
    [JsonPropertyName("BatteryStatus")] public string BatteryStatus { get; set; } = "";
    [JsonPropertyName("BatteryHealth")] public string BatteryHealth { get; set; } = "";
    [JsonPropertyName("UptimeSeconds")] public ulong UptimeSeconds { get; set; }

    /// <summary>Only when requested (a one-second sample); null otherwise.</summary>
    [JsonPropertyName("TopProcesses")] public IReadOnlyList<ProcessInfo>? TopProcesses { get; set; }
    [JsonPropertyName("HealthScore")] public int HealthScore { get; set; }
}

public sealed record DiskInfo
{
    [JsonPropertyName("Drive")] public string Drive { get; set; } = "";
    [JsonPropertyName("Total")] public ulong Total { get; set; }
    [JsonPropertyName("Free")] public ulong Free { get; set; }
    [JsonPropertyName("Used")] public ulong Used { get; set; }
}

public sealed record ProcessInfo
{
    [JsonPropertyName("Name")] public string Name { get; set; } = "";
    [JsonPropertyName("PID")] public int Pid { get; set; }
    [JsonPropertyName("CPU")] public double Cpu { get; set; }
    [JsonPropertyName("Memory")] public double Memory { get; set; }
    [JsonPropertyName("Status")] public string Status { get; set; } = "";
}

/// <summary>Reply to <c>doctor.run</c>.</summary>
public sealed record DoctorSnapshot
{
    public bool Healthy { get; set; }
    public int Passed { get; set; }
    public int Warnings { get; set; }
    public int Failed { get; set; }
    public IReadOnlyList<DoctorCheck> Results { get; set; } = [];
}

public sealed record DoctorCheck
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
    public string Status { get; set; } = "";
    public string Message { get; set; } = "";
    public string Description { get; set; } = "";
}

/// <summary>One clean category, as returned by <c>clean.scan</c> and <c>clean.run</c>.</summary>
public sealed record CleanCategory
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
    public string Description { get; set; } = "";
    public string Group { get; set; } = "";
    public long Bytes { get; set; }
    public int Files { get; set; }
    public bool AdminRequired { get; set; }
    public string Error { get; set; } = "";
}

public sealed record CleanResult
{
    public IReadOnlyList<CleanCategory> Categories { get; set; } = [];
    public long Bytes { get; set; }
    public int Files { get; set; }

    /// <summary>Set by the client, not the wire: the run was canceled and this is what finished first.</summary>
    [JsonIgnore] public bool Canceled { get; set; }
}

/// <summary>A <c>progress</c> event: <see cref="State"/> is "start" or "done".</summary>
public sealed record CleanProgress
{
    public string Category { get; set; } = "";
    public string State { get; set; } = "";
    public int Index { get; set; }
    public int Total { get; set; }
    public long Bytes { get; set; }
}

/// <summary>One quarantine session from <c>restore.list</c> (same shape as <c>du restore --json</c>).</summary>
public sealed record RestoreSession
{
    public int Number { get; set; }
    public string Id { get; set; } = "";
    public string Command { get; set; } = "";
    public DateTimeOffset Created { get; set; }
    public DateTimeOffset Expires { get; set; }
    public IReadOnlyList<RestoreItem> Items { get; set; } = [];
    public long Size { get; set; }
    public bool Damaged { get; set; }
}

public sealed record RestoreItem
{
    public int Number { get; set; }
    public string Path { get; set; } = "";
    public long Size { get; set; }
    public bool Dir { get; set; }
}

public sealed record RestoreSessionList
{
    public IReadOnlyList<RestoreSession> Sessions { get; set; } = [];

    /// <summary>What the 7-day expiry removed before listing (as <c>du restore</c> prints), or empty.</summary>
    public string Expired { get; set; } = "";
    public string Warning { get; set; } = "";
}

/// <summary>What happened to one item: <see cref="Status"/> is restored, skipped, failed, or (preview) would restore.</summary>
public sealed record RestoreOutcome
{
    public string Path { get; set; } = "";
    public long Size { get; set; }
    public string Status { get; set; } = "";
    public string Reason { get; set; } = "";
}

public sealed record RestoreRunResult
{
    public IReadOnlyList<RestoreOutcome> Results { get; set; } = [];
    public bool Failed { get; set; }
}

public sealed record RestoreEmptyResult
{
    public int Emptied { get; set; }
    public long Size { get; set; }
}

/// <summary>A folder or file from an analyze scan; <see cref="Id"/> is what the engine accepts back.</summary>
public sealed record AnalyzeItem
{
    public long Id { get; set; }
    public string Name { get; set; } = "";
    public string Path { get; set; } = "";
    public long Size { get; set; }
    public bool IsDir { get; set; }
    public int Items { get; set; }
}

/// <summary>A scanned folder: its largest entries (at most 500; <see cref="More"/> counts the rest) and largest files.</summary>
public sealed record AnalyzeFolder
{
    public long Id { get; set; }
    public string Path { get; set; } = "";
    public long Size { get; set; }
    public IReadOnlyList<AnalyzeItem> Entries { get; set; } = [];
    public int More { get; set; }
    public IReadOnlyList<AnalyzeItem> Largest { get; set; } = [];

    /// <summary>Breadcrumb from the scanned folder down to this folder's parent.</summary>
    public IReadOnlyList<AnalyzeCrumb> Trail { get; set; } = [];

    public string Name => System.IO.Path.GetFileName(Path.TrimEnd('\\', '/')) is { Length: > 0 } name ? name : Path;
}

public sealed record AnalyzeCrumb
{
    public long Id { get; set; }
    public string Path { get; set; } = "";
}

public sealed record AnalyzeResult
{
    public AnalyzeFolder Root { get; set; } = new();
    public int Dirs { get; set; }
    public int Files { get; set; }

    /// <summary>Null on the first scan of a folder.</summary>
    public ChangeReport? Changes { get; set; }
    public IReadOnlyList<string> HistoryNotes { get; set; } = [];
}

/// <summary>What changed since the previous scan (cmd/analyze_history.go changeReport).</summary>
public sealed record ChangeReport
{
    public string Path { get; set; } = "";
    public DateTimeOffset Since { get; set; }
    public long PreviousTotal { get; set; }
    public long Total { get; set; }
    public long Delta { get; set; }
    public IReadOnlyList<ChangeEntry> Entries { get; set; } = [];
}

public sealed record ChangeEntry
{
    /// <summary>The folder to open for this change (its parent, as the TUI's Enter); 0 when there is none.</summary>
    public long Id { get; set; }
    public string Path { get; set; } = "";
    public string Kind { get; set; } = "";
    public string Status { get; set; } = "";
    public long PreviousSize { get; set; }
    public long Size { get; set; }
    public long Delta { get; set; }
}

public sealed record AnalyzeProgress
{
    public int Dirs { get; set; }
    public int Files { get; set; }
    public long Bytes { get; set; }
    public string Path { get; set; } = "";
}

/// <summary><see cref="Kept"/>: the Recycle Bin refused it, so Duster's quarantine keeps it for 7 days.</summary>
public sealed record RecycleResult
{
    public bool Kept { get; set; }
    public long Bytes { get; set; }
}

/// <summary>A <c>progress</c> event of the parity methods: purge (found/index), optimize (task/state), vdisk (index).</summary>
public sealed record ItemProgress
{
    public int Index { get; set; }
    public int Total { get; set; }
    public int Found { get; set; }
    public string Path { get; set; } = "";
    public string Task { get; set; } = "";
    public string State { get; set; } = "";
}

/// <summary>Reply to <c>verify.run</c> (<c>du verify --json</c>).</summary>
public sealed record VerifyReport
{
    public bool Healthy { get; set; }
    public int Total { get; set; }
    public int Passed { get; set; }
    public int Failed { get; set; }
    public IReadOnlyList<VerifyCase> Cases { get; set; } = [];
}

public sealed record VerifyCase
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
    public bool Passed { get; set; }
    public string Details { get; set; } = "";
    public string Description { get; set; } = "";
}

/// <summary>Reply to <c>benchmark.run</c> (<c>du benchmark --json</c>).</summary>
public sealed record BenchmarkMetrics
{
    public double ScanFilesPerSec { get; set; }
    public int ScanFilesCount { get; set; }
    public long ScanDurationMs { get; set; }
    public double WriteOpsPerSec { get; set; }
    public long WriteDurationMs { get; set; }
    public double DeleteOpsPerSec { get; set; }
    public long DeleteDurationMs { get; set; }
    public ulong HeapAllocBytes { get; set; }
    public ulong HeapObjectsCount { get; set; }
    public int GoroutineCount { get; set; }
    public double CpuUsagePercent { get; set; }
    public double JsonSpeedPerSec { get; set; }
    public long JsonDurationMs { get; set; }
}

public sealed record SecurityReport
{
    public IReadOnlyList<SecurityCheck> Checks { get; set; } = [];
    public int Score { get; set; }
}

public sealed record SecurityCheck
{
    public string Name { get; set; } = "";
    public string Details { get; set; } = "";
    public string Status { get; set; } = "";
}

public sealed record DriverList
{
    public IReadOnlyList<DriverInfo> Drivers { get; set; } = [];
}

public sealed record DriverInfo
{
    public string Name { get; set; } = "";
    public string Version { get; set; } = "";
    public string Manufacturer { get; set; } = "";
    public bool Signed { get; set; }
    public string Class { get; set; } = "";
}

/// <summary>Reply to <c>oplog.list</c>: operations.log, newest first.</summary>
public sealed record OplogList
{
    public IReadOnlyList<OplogEntry> Entries { get; set; } = [];
    public int More { get; set; }
}

public sealed record OplogEntry
{
    public string Time { get; set; } = "";
    public string Command { get; set; } = "";
    public string Action { get; set; } = "";
    public string Target { get; set; } = "";
    public long Size { get; set; }
    public string Status { get; set; } = "";
}

public sealed record StartupList
{
    public IReadOnlyList<StartupEntry> Entries { get; set; } = [];
    public bool Admin { get; set; }
}

public sealed record StartupEntry
{
    public int Id { get; set; }
    public string Name { get; set; } = "";
    public string Command { get; set; } = "";
    public string Location { get; set; } = "";
    public bool Enabled { get; set; }
    public bool AdminRequired { get; set; }
}

public sealed record StartupToggleResult
{
    public bool Enabled { get; set; }
}

public sealed record StartupRemoveResult
{
    public int Removed { get; set; }
    public IReadOnlyList<string> Errors { get; set; } = [];
}

public sealed record PurgeScan
{
    public string Path { get; set; } = "";
    public IReadOnlyList<PurgeArtifact> Artifacts { get; set; } = [];
    public long Bytes { get; set; }
}

public sealed record PurgeArtifact
{
    public int Id { get; set; }
    public string Path { get; set; } = "";
    public string Name { get; set; } = "";
    public string Type { get; set; } = "";
    public string Framework { get; set; } = "";
    public long Size { get; set; }
    public bool Selected { get; set; }
}

/// <summary>Where purged items went: freed (permanent), recycled, or kept 7 days (never counted as freed).</summary>
public sealed record PurgeResult
{
    public long Freed { get; set; }
    public long Recycled { get; set; }
    public long Kept { get; set; }
    public int KeptCount { get; set; }
    public int Done { get; set; }
    public int Failed { get; set; }
    public IReadOnlyList<string> Errors { get; set; } = [];
    public string Notice { get; set; } = "";
}

public sealed record InstallerScan
{
    public IReadOnlyList<InstallerItem> Items { get; set; } = [];
}

public sealed record InstallerItem
{
    public int Id { get; set; }
    public string Path { get; set; } = "";
    public string Name { get; set; } = "";
    public long SizeBytes { get; set; }
    public int AgeDays { get; set; }
}

/// <summary>Installer and leftover sweeps: what was kept in the quarantine for 7 days.</summary>
public sealed record KeepResult
{
    public long Bytes { get; set; }
    public int Kept { get; set; }
    public int Failed { get; set; }
    public string Notice { get; set; } = "";
}

public sealed record AppList
{
    public IReadOnlyList<InstalledApp> Apps { get; set; } = [];
    public bool Admin { get; set; }
}

public sealed record InstalledApp
{
    public int Id { get; set; }
    public bool Protected { get; set; }
    public bool PerUser { get; set; }
    public string Name { get; set; } = "";
    public string Publisher { get; set; } = "";
    public string Version { get; set; } = "";
    public string InstallDate { get; set; } = "";
    public long Size { get; set; }
}

public sealed record UninstallResult
{
    public bool StillInstalled { get; set; }
    public IReadOnlyList<Leftover> Leftovers { get; set; } = [];
}

public sealed record Leftover
{
    public int Id { get; set; }
    public string Path { get; set; } = "";
    public long Size { get; set; }
}

public sealed record OptimizeList
{
    public IReadOnlyList<OptimizeTask> Tasks { get; set; } = [];
    public bool Admin { get; set; }
    public IReadOnlyList<ReclaimItem> Reclaim { get; set; } = [];
}

/// <summary>An optimize task; after a run <see cref="Status"/> is completed, failed or skipped.</summary>
public sealed record OptimizeTask
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
    public string Description { get; set; } = "";
    public bool AdminRequired { get; set; }
    public string Status { get; set; } = "";
    public long Reclaimed { get; set; }
    public string Note { get; set; } = "";
    public string Error { get; set; } = "";
}

public sealed record OptimizeResult
{
    public IReadOnlyList<OptimizeTask> Tasks { get; set; } = [];
    public long Reclaimed { get; set; }
}

/// <summary>Windows.old or the hibernation file: reported with Windows' own advice, never removed.</summary>
public sealed record ReclaimItem
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
    public string Path { get; set; } = "";
    public bool Present { get; set; }
    public long Bytes { get; set; }
    public bool Partial { get; set; }
    public IReadOnlyList<string> Hints { get; set; } = [];
    public string Advice { get; set; } = "";
}

public sealed record VirtualDiskList
{
    public IReadOnlyList<VirtualDisk> Disks { get; set; } = [];
    public bool Admin { get; set; }
    public IReadOnlyList<string> Advice { get; set; } = [];
}

public sealed record VirtualDisk
{
    public int Id { get; set; }
    public string Kind { get; set; } = "";
    public string Label { get; set; } = "";
    public string Path { get; set; } = "";
    public long Bytes { get; set; }
    public long OnDiskBytes { get; set; }
    public long UsedBytes { get; set; }
    public bool UsedKnown { get; set; }
    public bool Sparse { get; set; }
    public bool Compressed { get; set; }
    public bool Encrypted { get; set; }

    /// <summary>Why this disk cannot be compacted (sparse, compressed, encrypted), or empty.</summary>
    public string Blocked { get; set; } = "";
    public long Estimate { get; set; }
    public bool EstimateKnown { get; set; }
}

public sealed record VirtualDiskResult
{
    public IReadOnlyList<VirtualDiskOutcome> Results { get; set; } = [];
    public long Freed { get; set; }
    public string Warning { get; set; } = "";
}

public sealed record VirtualDiskOutcome
{
    public string Path { get; set; } = "";
    public string Label { get; set; } = "";
    public string Status { get; set; } = "";
    public long FreedBytes { get; set; }
    public string Note { get; set; } = "";
}

public sealed record ScheduleInfo
{
    public ScheduleStatus Status { get; set; } = new();
    public IReadOnlyList<Choice> Safe { get; set; } = [];
    public IReadOnlyList<Choice> OptIn { get; set; } = [];
}

public sealed record Choice
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
}

/// <summary><c>du schedule status --json</c>.</summary>
public sealed record ScheduleStatus
{
    public bool Enabled { get; set; }
    public string TaskName { get; set; } = "";
    public string Every { get; set; } = "";
    public string At { get; set; } = "";

    /// <summary>Null when low-space cleaning is off.</summary>
    public int? LowSpacePercent { get; set; }
    public IReadOnlyList<string> Categories { get; set; } = [];
    public DateTimeOffset? NextCheck { get; set; }
    public ScheduleCheck? LastCheck { get; set; }
    public ScheduleClean? LastClean { get; set; }
    public IReadOnlyList<string> Warnings { get; set; } = [];
    public IReadOnlyList<string>? Notes { get; set; }
    public bool DryRun { get; set; }
    public IReadOnlyList<ScheduleCategoryResult>? WouldClean { get; set; }
}

public sealed record ScheduleCheck
{
    public DateTimeOffset Time { get; set; }
    public string Result { get; set; } = "";
    public double FreePercent { get; set; }
}

public sealed record ScheduleClean
{
    public DateTimeOffset Time { get; set; }
    public string Reason { get; set; } = "";
    public long Freed { get; set; }
    public IReadOnlyList<ScheduleCategoryResult> Categories { get; set; } = [];
}

public sealed record ScheduleCategoryResult
{
    public string Id { get; set; } = "";
    public long Freed { get; set; }
    public int Files { get; set; }
    public string Status { get; set; } = "";
    public string Error { get; set; } = "";
}

public sealed record ScheduleOffResult
{
    public bool AlreadyOff { get; set; }
}

public sealed record UpdateInfo
{
    public string Current { get; set; } = "";
    public string Latest { get; set; } = "";
    public bool Available { get; set; }
    public string PublishedAt { get; set; } = "";
    public string Notes { get; set; } = "";
}

public sealed record UpdateInstallResult
{
    public string Version { get; set; } = "";
    public bool GuiUpdated { get; set; }
}

/// <summary>What <c>du remove</c> would delete.</summary>
public sealed record RemovePlan
{
    public string Exe { get; set; } = "";
    public string DataDir { get; set; } = "";
    public long KeptBytes { get; set; }
    public bool Protected { get; set; }

    /// <summary>The setup's uninstaller sits beside Duster: Windows Settings > Apps removes it properly.</summary>
    public bool SetupInstalled { get; set; }
}

/// <summary>Replies that carry nothing.</summary>
public sealed record Empty;
