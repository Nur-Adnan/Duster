using System.Text.Json.Serialization;

namespace Duster.Core;

// Protocol DTOs for `du engine` (cmd/engine.go). Engine types use snake_case
// names; SystemStats mirrors lib/sysinfo, which has no json tags, so its
// properties carry the Go field names explicitly.

/// <summary>Reply to <c>hello</c>.</summary>
public sealed record EngineHello
{
    public int Protocol { get; init; }
    public string Version { get; init; } = "";
    public bool Admin { get; init; }
}

/// <summary>Reply to <c>status.get</c> (the fields the GUI shows).</summary>
public sealed record SystemStats
{
    [JsonPropertyName("HostName")] public string HostName { get; init; } = "";
    [JsonPropertyName("OSVersion")] public string OSVersion { get; init; } = "";
    [JsonPropertyName("CPUModel")] public string CPUModel { get; init; } = "";
    [JsonPropertyName("CPUPercent")] public double CPUPercent { get; init; }
    [JsonPropertyName("RAMTotal")] public ulong RAMTotal { get; init; }
    [JsonPropertyName("RAMUsed")] public ulong RAMUsed { get; init; }
    [JsonPropertyName("Disks")] public IReadOnlyList<DiskInfo> Disks { get; init; } = [];
}

public sealed record DiskInfo
{
    [JsonPropertyName("Drive")] public string Drive { get; init; } = "";
    [JsonPropertyName("Total")] public ulong Total { get; init; }
    [JsonPropertyName("Free")] public ulong Free { get; init; }
}

/// <summary>Reply to <c>doctor.run</c>.</summary>
public sealed record DoctorSnapshot
{
    public bool Healthy { get; init; }
    public int Passed { get; init; }
    public int Warnings { get; init; }
    public int Failed { get; init; }
    public IReadOnlyList<DoctorCheck> Results { get; init; } = [];
}

public sealed record DoctorCheck
{
    public string Id { get; init; } = "";
    public string Name { get; init; } = "";
    public string Status { get; init; } = "";
    public string Message { get; init; } = "";
}

/// <summary>One clean category, as returned by <c>clean.scan</c> and <c>clean.run</c>.</summary>
public sealed record CleanCategory
{
    public string Id { get; init; } = "";
    public string Name { get; init; } = "";
    public string Description { get; init; } = "";
    public string Group { get; init; } = "";
    public long Bytes { get; init; }
    public int Files { get; init; }
    public bool AdminRequired { get; init; }
    public string Error { get; init; } = "";
}

public sealed record CleanResult
{
    public IReadOnlyList<CleanCategory> Categories { get; init; } = [];
    public long Bytes { get; init; }
    public int Files { get; init; }

    /// <summary>Set by the client, not the wire: the run was canceled and this is what finished first.</summary>
    [JsonIgnore] public bool Canceled { get; init; }
}

/// <summary>A <c>progress</c> event: <see cref="State"/> is "start" or "done".</summary>
public sealed record CleanProgress
{
    public string Category { get; init; } = "";
    public string State { get; init; } = "";
    public int Index { get; init; }
    public int Total { get; init; }
    public long Bytes { get; init; }
}

/// <summary>One quarantine session from <c>restore.list</c> (same shape as <c>du restore --json</c>).</summary>
public sealed record RestoreSession
{
    public int Number { get; init; }
    public string Id { get; init; } = "";
    public string Command { get; init; } = "";
    public DateTimeOffset Created { get; init; }
    public DateTimeOffset Expires { get; init; }
    public IReadOnlyList<RestoreItem> Items { get; init; } = [];
    public long Size { get; init; }
    public bool Damaged { get; init; }
}

public sealed record RestoreItem
{
    public int Number { get; init; }
    public string Path { get; init; } = "";
    public long Size { get; init; }
    public bool Dir { get; init; }
}

public sealed record RestoreSessionList
{
    public IReadOnlyList<RestoreSession> Sessions { get; init; } = [];
}

/// <summary>What happened to one item: <see cref="Status"/> is restored, skipped or failed.</summary>
public sealed record RestoreOutcome
{
    public string Path { get; init; } = "";
    public long Size { get; init; }
    public string Status { get; init; } = "";
    public string Reason { get; init; } = "";
}

public sealed record RestoreRunResult
{
    public IReadOnlyList<RestoreOutcome> Results { get; init; } = [];
    public bool Failed { get; init; }
}

public sealed record RestoreEmptyResult
{
    public int Emptied { get; init; }
    public long Size { get; init; }
}

/// <summary>A folder or file from an analyze scan; <see cref="Id"/> is what the engine accepts back.</summary>
public sealed record AnalyzeItem
{
    public long Id { get; init; }
    public string Name { get; init; } = "";
    public string Path { get; init; } = "";
    public long Size { get; init; }
    public bool IsDir { get; init; }
    public int Items { get; init; }
}

/// <summary>A scanned folder: its largest entries (at most 500; <see cref="More"/> counts the rest) and largest files.</summary>
public sealed record AnalyzeFolder
{
    public long Id { get; init; }
    public string Path { get; init; } = "";
    public long Size { get; init; }
    public IReadOnlyList<AnalyzeItem> Entries { get; init; } = [];
    public int More { get; init; }
    public IReadOnlyList<AnalyzeItem> Largest { get; init; } = [];

    public string Name => System.IO.Path.GetFileName(Path.TrimEnd('\\', '/')) is { Length: > 0 } name ? name : Path;
}

public sealed record AnalyzeResult
{
    public AnalyzeFolder Root { get; init; } = new();
    public int Dirs { get; init; }
    public int Files { get; init; }

    /// <summary>Null on the first scan of a folder.</summary>
    public ChangeReport? Changes { get; init; }
    public IReadOnlyList<string> HistoryNotes { get; init; } = [];
}

/// <summary>What changed since the previous scan (cmd/analyze_history.go changeReport).</summary>
public sealed record ChangeReport
{
    public string Path { get; init; } = "";
    public DateTimeOffset Since { get; init; }
    public long PreviousTotal { get; init; }
    public long Total { get; init; }
    public long Delta { get; init; }
    public IReadOnlyList<ChangeEntry> Entries { get; init; } = [];
}

public sealed record ChangeEntry
{
    public string Path { get; init; } = "";
    public string Kind { get; init; } = "";
    public string Status { get; init; } = "";
    public long PreviousSize { get; init; }
    public long Size { get; init; }
    public long Delta { get; init; }
}

public sealed record AnalyzeProgress
{
    public int Dirs { get; init; }
    public int Files { get; init; }
    public long Bytes { get; init; }
    public string Path { get; init; } = "";
}

/// <summary><see cref="Kept"/>: the Recycle Bin refused it, so Duster's quarantine keeps it for 7 days.</summary>
public sealed record RecycleResult
{
    public bool Kept { get; init; }
    public long Bytes { get; init; }
}
