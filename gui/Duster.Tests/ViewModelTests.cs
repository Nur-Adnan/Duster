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
        var engine = new FakeEngine { Connected = true };
        engine.Replies["restore.list"] = new RestoreSessionList { Sessions = [session] };
        engine.Replies["restore.run"] = new RestoreRunResult { Results = [new() { Status = "restored", Size = 6 }, new() { Status = "skipped" }] };
        var host = new FakeHost { Confirm = false };
        var vm = new RestoreViewModel(engine, host);
        await vm.RefreshCommand.ExecuteAsync(null);
        Assert.AreEqual(session, vm.SelectedSession);

        await vm.RestoreSessionCommand.ExecuteAsync(null);
        Assert.AreEqual("""{"id":"s1","item":0,"dry_run":false}""", engine.Sent("restore.run"));
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
}
