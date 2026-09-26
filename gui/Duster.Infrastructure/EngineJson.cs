using System.Text.Json.Serialization;
using Duster.Core;

namespace Duster.Infrastructure;

// Source-generated so the GUI stays trim- and AOT-ready.
[JsonSourceGenerationOptions(PropertyNamingPolicy = JsonKnownNamingPolicy.SnakeCaseLower)]
[JsonSerializable(typeof(EngineHello))]
[JsonSerializable(typeof(SystemStats))]
[JsonSerializable(typeof(DoctorSnapshot))]
[JsonSerializable(typeof(CleanResult))]
[JsonSerializable(typeof(CleanProgress))]
internal sealed partial class EngineJson : JsonSerializerContext;
