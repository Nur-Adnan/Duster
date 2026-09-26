using Duster.Core;
using Duster.Core.ViewModels;

namespace Duster.Tests;

[TestClass]
public sealed class ViewModelTests
{
    private static CleanCategory Cat(string id, string group, long bytes, bool admin = false) =>
        new() { Id = id, Name = id, Group = group, Bytes = bytes, AdminRequired = admin };

    private static CleanResult ScanResult() => new()
    {
        Categories = [Cat("temp", "System Core", 100), Cat("prefetch", "System Core", 50, admin: true), Cat("npm", "Developer Tools", 30)],
        Bytes = 180,
    };

    [TestMethod]
    public async Task ScanGroupsCategoriesAndSkipsAdminOnlyFromTheDefaultSelection()
    {
        var engine = new FakeEngine { Scan = ScanResult() };
        var vm = new CleanViewModel(engine, new FakeHost());
        await vm.ScanCommand.ExecuteAsync(null);

        CollectionAssert.AreEqual(new[] { "System Core", "Developer Tools" }, vm.Groups.Select(g => g.Name).ToArray());
        Assert.IsTrue(vm.NeedsAdmin);
        Assert.AreEqual(2, vm.SelectedCount, "prefetch needs admin, so it starts unselected");
        Assert.IsFalse(vm.Groups[0].Single(i => i.Category.Id == "prefetch").IsSelected);
    }

    [TestMethod]
    public async Task CleanDoesNothingUntilTheUserConfirms()
    {
        var engine = new FakeEngine { Scan = ScanResult() };
        var host = new FakeHost { Confirm = false };
        var vm = new CleanViewModel(engine, host);
        await vm.ScanCommand.ExecuteAsync(null);
        await vm.CleanCommand.ExecuteAsync(null);

        Assert.IsNull(engine.CleanedIds, "declining the confirm dialog still reached the engine");
        StringAssert.Contains(host.LastMessage, "permanently delete");
        StringAssert.Contains(host.LastMessage, "can't be undone");
    }

    [TestMethod]
    public async Task CleanSendsOnlySelectedSelectableCategoriesAndShowsOutcomes()
    {
        var engine = new FakeEngine
        {
            Scan = ScanResult(),
            CleanResult = new() { Categories = [Cat("temp", "System Core", 100) with { Error = "" }], Bytes = 100, Files = 3 },
        };
        var vm = new CleanViewModel(engine, new FakeHost());
        await vm.ScanCommand.ExecuteAsync(null);
        vm.Groups[1][0].IsSelected = false; // npm
        vm.Groups[0].Single(i => i.Category.Id == "prefetch").IsSelected = true; // not selectable: must not be sent

        await vm.CleanCommand.ExecuteAsync(null);

        CollectionAssert.AreEqual(new[] { "temp" }, engine.CleanedIds!.ToArray());
        Assert.AreEqual("Freed 100 B", vm.Groups[0][0].Outcome);
        StringAssert.StartsWith(vm.Status, "Freed 100 B");
    }

    [TestMethod]
    public async Task CanceledCleanReportsWhatFinishedAndMarksTheRest()
    {
        var engine = new FakeEngine
        {
            Scan = ScanResult(),
            CleanResult = new() { Categories = [Cat("temp", "System Core", 100)], Bytes = 100, Canceled = true },
        };
        var vm = new CleanViewModel(engine, new FakeHost());
        await vm.ScanCommand.ExecuteAsync(null);
        await vm.CleanCommand.ExecuteAsync(null);

        StringAssert.StartsWith(vm.Status, "Canceled.");
        Assert.AreEqual("Not cleaned", vm.Groups[1][0].Outcome);
    }

    [TestMethod]
    public void DeclinedAdministratorRestartSaysSo()
    {
        var vm = new CleanViewModel(new FakeEngine(), new FakeHost { Elevate = false });
        vm.RestartAsAdministratorCommand.Execute(null);
        StringAssert.Contains(vm.Status, "canceled");
    }

    [TestMethod]
    public async Task RestoreListsRestoresAndEmptiesOnlyAfterConfirming()
    {
        var session = new RestoreSession { Id = "s1", Command = "purge", Size = 10, Items = [new() { Number = 1, Path = "/p", Size = 10 }] };
        var engine = new FakeEngine
        {
            Connected = true,
            Sessions = [session],
            Restored = new() { Results = [new() { Status = "restored", Size = 6 }, new() { Status = "skipped" }] },
        };
        var host = new FakeHost { Confirm = false };
        var vm = new RestoreViewModel(engine, host);
        await vm.RefreshCommand.ExecuteAsync(null);
        Assert.AreEqual(session, vm.SelectedSession);

        await vm.RestoreSessionCommand.ExecuteAsync(null);
        Assert.AreEqual(("s1", 0), engine.RestoreCall);
        StringAssert.StartsWith(vm.Status, "Restored 1 item (6 B); skipped 1");

        await vm.EmptySessionCommand.ExecuteAsync(null);
        Assert.IsNull(engine.EmptiedIds, "declining the confirm dialog still emptied");
        host.Confirm = true;
        await vm.EmptySessionCommand.ExecuteAsync(null);
        CollectionAssert.AreEqual(new[] { "s1" }, engine.EmptiedIds!.ToArray());
    }

    [TestMethod]
    public async Task AnalyzeDrillsDownGoesBackAndRecyclesWithoutCallingQuarantineFreed()
    {
        var sub = new AnalyzeItem { Id = 2, Name = "sub", IsDir = true, Size = 60 };
        var file = new AnalyzeItem { Id = 3, Name = "f.bin", Size = 40 };
        var root = new AnalyzeFolder { Id = 1, Path = "/r", Size = 100, Entries = [sub, file] };
        var engine = new FakeEngine
        {
            Analysis = new() { Root = root, Files = 1 },
            Folders = { [1] = root, [2] = new AnalyzeFolder { Id = 2, Path = "/r/sub", Size = 60 } },
            Recycled = new() { Kept = true, Bytes = 40 },
        };
        var vm = new AnalyzeViewModel(engine, new FakeHost());
        await vm.ScanCommand.ExecuteAsync(null);
        Assert.AreEqual(60, vm.Entries[0].Percent);

        await vm.OpenCommand.ExecuteAsync(vm.Entries[1]); // a file: ignored
        Assert.HasCount(1, vm.Trail);
        await vm.OpenCommand.ExecuteAsync(vm.Entries[0]);
        Assert.AreEqual("/r/sub", vm.Current!.Path);
        await vm.GoToCommand.ExecuteAsync(vm.Trail[0]);
        Assert.HasCount(1, vm.Trail);

        vm.SelectedRow = vm.Entries[1];
        await vm.RecycleCommand.ExecuteAsync(null);
        Assert.AreEqual(3, engine.RecycledId);
        StringAssert.Contains(vm.Status, "kept it for 7 days");
        Assert.IsFalse(vm.Status.Contains("freed", StringComparison.OrdinalIgnoreCase));
    }

    private sealed class FakeHost : IAppHost
    {
        public bool Confirm { get; set; } = true;
        public bool Elevate { get; init; } = true;
        public string LastMessage { get; private set; } = "";

        public Task<bool> ConfirmAsync(string title, string message, string confirmLabel)
        {
            LastMessage = message;
            return Task.FromResult(Confirm);
        }

        public bool RestartAsAdministrator() => Elevate;

        public Task<string?> PickFolderAsync() => Task.FromResult<string?>(null);
    }

    private sealed class FakeEngine : IEngineClient
    {
        public bool Connected { get; init; }
        public CleanResult Scan { get; init; } = new();
        public CleanResult CleanResult { get; init; } = new();
        public IReadOnlyList<RestoreSession> Sessions { get; init; } = [];
        public RestoreRunResult Restored { get; init; } = new();
        public AnalyzeResult Analysis { get; init; } = new();
        public Dictionary<long, AnalyzeFolder> Folders { get; } = [];
        public RecycleResult Recycled { get; init; } = new();

        public IReadOnlyCollection<string>? CleanedIds { get; private set; }
        public (string, int)? RestoreCall { get; private set; }
        public IReadOnlyCollection<string>? EmptiedIds { get; private set; }
        public long? RecycledId { get; private set; }

        public EngineState State => Connected ? EngineState.Connected : EngineState.Stopped;
        public EngineException? Fault => null;
        public EngineHello? Hello => null;
        public event EventHandler? StateChanged { add { } remove { } }

        public Task StartAsync(CancellationToken ct = default) => Task.CompletedTask;
        public Task<SystemStats> GetStatusAsync(CancellationToken ct = default) => Task.FromResult(new SystemStats());
        public Task<DoctorSnapshot> RunDoctorAsync(CancellationToken ct = default) => Task.FromResult(new DoctorSnapshot());
        public Task<CleanResult> ScanCleanAsync(IProgress<CleanProgress>? progress, CancellationToken ct = default) => Task.FromResult(Scan);

        public Task<CleanResult> RunCleanAsync(IReadOnlyCollection<string> ids, IProgress<CleanProgress>? progress, CancellationToken ct = default)
        {
            CleanedIds = ids;
            return Task.FromResult(CleanResult);
        }

        public Task<IReadOnlyList<RestoreSession>> ListRestoreAsync(CancellationToken ct = default) => Task.FromResult(Sessions);

        public Task<RestoreRunResult> RestoreAsync(string sessionId, int item, CancellationToken ct = default)
        {
            RestoreCall = (sessionId, item);
            return Task.FromResult(Restored);
        }

        public Task<RestoreEmptyResult> EmptyRestoreAsync(IReadOnlyCollection<string> sessionIds, CancellationToken ct = default)
        {
            EmptiedIds = sessionIds;
            return Task.FromResult(new RestoreEmptyResult { Emptied = sessionIds.Count });
        }

        public Task<AnalyzeResult> AnalyzeAsync(string path, IProgress<AnalyzeProgress>? progress, CancellationToken ct = default) => Task.FromResult(Analysis);
        public Task<AnalyzeFolder> AnalyzeChildrenAsync(long id, CancellationToken ct = default) => Task.FromResult(Folders[id]);

        public Task<RecycleResult> RecycleAsync(long id, CancellationToken ct = default)
        {
            RecycledId = id;
            return Task.FromResult(Recycled);
        }

        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }
}
