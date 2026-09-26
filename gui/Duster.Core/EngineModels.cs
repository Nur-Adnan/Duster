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

/// <summary>Reply to <c>status.get</c> (the fields the GUI shows).</summary>
public sealed record SystemStats
{
    [JsonPropertyName("HostName")] public string HostName { get; set; } = "";
    [JsonPropertyName("OSVersion")] public string OSVersion { get; set; } = "";
    [JsonPropertyName("CPUModel")] public string CPUModel { get; set; } = "";
    [JsonPropertyName("CPUPercent")] public double CPUPercent { get; set; }
    [JsonPropertyName("RAMTotal")] public ulong RAMTotal { get; set; }
    [JsonPropertyName("RAMUsed")] public ulong RAMUsed { get; set; }
    [JsonPropertyName("Disks")] public IReadOnlyList<DiskInfo> Disks { get; set; } = [];
}

public sealed record DiskInfo
{
    [JsonPropertyName("Drive")] public string Drive { get; set; } = "";
    [JsonPropertyName("Total")] public ulong Total { get; set; }
    [JsonPropertyName("Free")] public ulong Free { get; set; }
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
}

/// <summary>What happened to one item: <see cref="Status"/> is restored, skipped or failed.</summary>
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

    public string Name => System.IO.Path.GetFileName(Path.TrimEnd('\\', '/')) is { Length: > 0 } name ? name : Path;
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
