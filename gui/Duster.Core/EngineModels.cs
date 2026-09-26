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
