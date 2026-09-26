using System.Collections.ObjectModel;
using System.Text;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

/// <summary>One check result: <see cref="Status"/> is PASS, WARN, FAIL or SKIPPED.</summary>
public sealed record CheckRow(string Name, string Status, string Message, string Description);

/// <summary>
/// <c>du doctor</c>, <c>du verify</c> and <c>du benchmark</c>. Each runs only
/// when asked; the results can be copied as a report.
/// </summary>
public sealed partial class DiagnosticsViewModel(IEngineClient engine, IAppHost host) : ToolViewModel(engine, host)
{
    public ObservableCollection<CheckRow> Health { get; } = [];
    public ObservableCollection<CheckRow> SelfTest { get; } = [];
    public ObservableCollection<StatRow> Benchmark { get; } = [];

    [ObservableProperty]
    public partial string HealthSummary { get; set; } = "Checks privileges, temp folders, PowerShell, Windows version, Defender, caches, long paths, junction loops, the terminal and the install location. Changes nothing.";

    [ObservableProperty]
    public partial string SelfTestSummary { get; set; } = "Runs Duster's 8 safety self-tests in temporary folders: protected paths, link guards, dry runs, the Recycle Bin fallback, registry safety and checksums.";

    [ObservableProperty]
    public partial string BenchmarkSummary { get; set; } = "Measures scan, write, delete and JSON speed in a temporary folder. It keeps the disk busy for a few seconds.";

    /// <summary>The privilege check warned: some checks and tasks need administrator rights.</summary>
    [ObservableProperty]
    public partial bool CanElevate { get; set; }

    [RelayCommand]
    private Task RunHealthCheckAsync() => RunAsync(async ct =>
    {
        HealthSummary = "Checking…";
        var r = await Engine.CallAsync(Calls.Doctor(), null, ct);
        Health.Clear();
        foreach (var c in r.Results)
        {
            Health.Add(new CheckRow(c.Name, c.Status, c.Message, c.Description));
        }
        CanElevate = r.Results.Any(c => c.Id == "privilege" && c.Status != "PASS");
        HealthSummary = $"{r.Passed} passed, {Format.Plural(r.Warnings, "warning", "warnings")}, {r.Failed} failed.";
    });

    [RelayCommand]
    private Task RunSelfTestAsync() => RunAsync(async ct =>
    {
        SelfTestSummary = "Testing…";
        var r = await Engine.CallAsync(Calls.Verify(), null, ct);
        SelfTest.Clear();
        foreach (var c in r.Cases)
        {
            SelfTest.Add(new CheckRow(c.Name, c.Passed ? "PASS" : "FAIL", c.Details, c.Description));
        }
        SelfTestSummary = $"{r.Passed} of {r.Total} passed" + (r.Failed > 0 ? $"; {r.Failed} failed: see each test." : ".");
    });

    [RelayCommand]
    private Task RunBenchmarkAsync() => RunAsync(async ct =>
    {
        BenchmarkSummary = "Running…";
        var m = await Engine.CallAsync(Calls.Benchmark(), null, ct);
        Benchmark.Clear();
        Benchmark.Add(new StatRow("Scan", $"{m.ScanFilesPerSec:N0} files/s ({m.ScanFilesCount:N0} files in {m.ScanDurationMs:N0} ms)"));
        Benchmark.Add(new StatRow("Write", $"{m.WriteOpsPerSec:N0} files/s ({m.WriteDurationMs:N0} ms)"));
        Benchmark.Add(new StatRow("Delete", $"{m.DeleteOpsPerSec:N0} files/s ({m.DeleteDurationMs:N0} ms)"));
        Benchmark.Add(new StatRow("JSON", $"{m.JsonSpeedPerSec:N0} records/s ({m.JsonDurationMs:N0} ms)"));
        Benchmark.Add(new StatRow("Engine memory", $"{Format.Bytes(m.HeapAllocBytes)} in {m.HeapObjectsCount:N0} objects, {m.GoroutineCount} goroutines"));
        Benchmark.Add(new StatRow("CPU", $"{m.CpuUsagePercent:0.#}%"));
        BenchmarkSummary = "Done.";
    });

    /// <summary>Everything run so far, as text (the GUI's <c>--json</c> for these commands).</summary>
    [RelayCommand]
    private void CopyReport()
    {
        var sb = new StringBuilder($"Duster diagnostics, {DateTime.Now:g}\n");
        void Section(string title, IEnumerable<CheckRow> rows)
        {
            if (rows.Any())
            {
                sb.Append($"\n{title}\n");
                foreach (var r in rows)
                {
                    sb.Append($"[{r.Status}] {r.Name}: {r.Message}\n");
                }
            }
        }
        Section("Health check", Health);
        Section("Self-test", SelfTest);
        if (Benchmark.Count > 0)
        {
            sb.Append("\nBenchmark\n");
            foreach (var r in Benchmark)
            {
                sb.Append($"{r.Label}: {r.Value}\n");
            }
        }
        Host.CopyText(sb.ToString());
        Status = "Report copied.";
    }
}
