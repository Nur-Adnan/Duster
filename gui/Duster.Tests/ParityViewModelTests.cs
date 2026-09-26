using System.Text.Json;
using Duster.Core;
using Duster.Core.ViewModels;

namespace Duster.Tests;

/// <summary>
/// The parity pages' safety rules: nothing changes without a confirm, only
/// chosen and allowed IDs reach the engine, in the CLI's formats, and results
/// never call kept bytes freed.
/// </summary>
[TestClass]
public sealed class ParityViewModelTests
{
    private static EngineException Canceled(string partialJson) =>
        new(EngineErrorKind.Canceled, "canceled", "canceled", JsonDocument.Parse(partialJson).RootElement.Clone());

    [TestMethod]
    public async Task PurgeAsksPerModeSendsOnlyChosenIdsAndNeverCallsKeptFreed()
    {
        var engine = new FakeEngine();
        engine.Replies["purge.scan"] = new PurgeScan
        {
            Artifacts = [new() { Id = 1, Path = "/a/node_modules", Size = 10, Selected = true }, new() { Id = 2, Path = "/b/target", Size = 5, Selected = true }],
            Bytes = 15,
        };
        engine.Replies["purge.run"] = new PurgeResult { Kept = 10, KeptCount = 1, Done = 1 };
        var host = new FakeHost { Confirm = false };
        var vm = new PurgeViewModel(engine, host) { Path = "/" };
        await vm.ScanCommand.ExecuteAsync(null);
        Assert.AreEqual("2 selected · 15 B", vm.SelectedText);

        vm.Items[1].IsSelected = false;
        vm.Mode = PurgeViewModel.Permanent;
        await vm.PurgeCommand.ExecuteAsync(null);
        Assert.IsFalse(engine.Asked("purge.run"), "declining still purged");
        StringAssert.Contains(host.LastMessage, "for good");
        StringAssert.Contains(host.LastMessage, "cannot be restored");

        host.Confirm = true;
        vm.Mode = PurgeViewModel.Keep;
        await vm.PurgeCommand.ExecuteAsync(null);
        Assert.AreEqual("""{"ids":[1],"mode":"keep"}""", engine.Sent("purge.run"));
        StringAssert.Contains(vm.Status, "Kept 10 B for 7 days");
        Assert.IsFalse(vm.Status.Contains("freed", StringComparison.OrdinalIgnoreCase));
        Assert.HasCount(1, vm.Items, "the purged folder is gone from the list");
    }

    [TestMethod]
    public async Task StoppedPurgeReportsWhatFinishedAndKeepsFailuresListed()
    {
        var engine = new FakeEngine();
        engine.Replies["purge.scan"] = new PurgeScan
        {
            Artifacts = [new() { Id = 1, Path = "/a", Size = 1, Selected = true }, new() { Id = 2, Path = "/b", Size = 2, Selected = true }, new() { Id = 3, Path = "/c", Size = 3, Selected = true }],
        };
        engine.Failures["purge.run"] = Canceled("""{"freed":1,"done":2,"failed":1,"errors":["/b: in use"]}""");
        var vm = new PurgeViewModel(engine, new FakeHost()) { Path = "/", Mode = PurgeViewModel.Permanent };
        await vm.ScanCommand.ExecuteAsync(null);
        await vm.PurgeCommand.ExecuteAsync(null);

        StringAssert.StartsWith(vm.Status, "Stopped. Freed 1 B (deleted for good); 1 failed");
        CollectionAssert.AreEqual(new[] { "/b", "/c" }, vm.Items.Select(r => r.Artifact.Path).ToArray());
        Assert.AreEqual("in use", vm.Items[0].Outcome);
    }

    [TestMethod]
    public async Task InstallersKeepOnlyTheChosenAndRescan()
    {
        var engine = new FakeEngine();
        engine.Replies["installer.scan"] = new InstallerScan { Items = [new() { Id = 1, Name = "a.exe", SizeBytes = 5 }, new() { Id = 2, Name = "b.msi", SizeBytes = 7 }] };
        engine.Replies["installer.run"] = new KeepResult { Bytes = 7, Kept = 1 };
        var vm = new InstallersViewModel(engine, new FakeHost()) { MinSizeMb = 0 };
        await vm.ScanCommand.ExecuteAsync(null);
        Assert.AreEqual("""{"min_size_mb":1}""", engine.Sent("installer.scan"), "--min-size is at least 1 MB");

        vm.Items[0].IsSelected = false;
        await vm.KeepCommand.ExecuteAsync(null);
        Assert.AreEqual("""{"ids":[2]}""", engine.Sent("installer.run"));
        Assert.AreEqual(2, engine.Calls.Count(c => c.Method == "installer.scan"), "the list is refreshed after a sweep");
        StringAssert.StartsWith(vm.Status, "Kept 7 B in 1 installer for 7 days");
    }

    [TestMethod]
    public async Task AppsRefuseProtectedAppsAndOfferLeftoversUnselected()
    {
        var engine = new FakeEngine();
        engine.Replies["uninstall.list"] = new AppList { Apps = [new() { Id = 1, Name = "Microsoft Visual C++", Protected = true }, new() { Id = 2, Name = "Zed" }] };
        engine.Replies["uninstall.run"] = new UninstallResult { Leftovers = [new() { Id = 1, Path = "/l/zed", Size = 4 }, new() { Id = 2, Path = "/l/zed2", Size = 6 }] };
        engine.Replies["uninstall.sweep"] = new KeepResult { Bytes = 6, Kept = 1 };
        var host = new FakeHost();
        var vm = new AppsViewModel(engine, host);
        await vm.LoadCommand.ExecuteAsync(null);

        vm.SelectedApp = vm.Apps[0];
        Assert.IsFalse(vm.UninstallCommand.CanExecute(null), "a protected app can be uninstalled");
        vm.Search = "ze";
        Assert.HasCount(1, vm.Apps);
        vm.SelectedApp = vm.Apps[0];

        host.Confirm = false;
        await vm.UninstallCommand.ExecuteAsync(null);
        Assert.IsFalse(engine.Asked("uninstall.run"), "declining still ran the uninstaller");

        host.Confirm = true;
        await vm.UninstallCommand.ExecuteAsync(null);
        Assert.AreEqual("""{"id":2}""", engine.Sent("uninstall.run"));
        Assert.IsTrue(vm.Leftovers.All(l => !l.IsSelected), "leftovers are matched by name, so none may start selected");
        Assert.IsFalse(vm.KeepLeftoversCommand.CanExecute(null));

        vm.Leftovers[1].IsSelected = true;
        await vm.KeepLeftoversCommand.ExecuteAsync(null);
        Assert.AreEqual("""{"ids":[2]}""", engine.Sent("uninstall.sweep"));
        StringAssert.StartsWith(vm.Status, "Kept 6 B for 7 days");
    }

    [TestMethod]
    public async Task AppStillInstalledOffersNoLeftovers()
    {
        var engine = new FakeEngine();
        engine.Replies["uninstall.list"] = new AppList { Apps = [new() { Id = 1, Name = "Zed" }] };
        engine.Replies["uninstall.run"] = new UninstallResult { StillInstalled = true };
        var vm = new AppsViewModel(engine, new FakeHost());
        await vm.LoadCommand.ExecuteAsync(null);
        vm.SelectedApp = vm.Apps[0];
        await vm.UninstallCommand.ExecuteAsync(null);
        Assert.IsEmpty(vm.Leftovers);
        StringAssert.Contains(vm.Status, "still installed");
    }

    [TestMethod]
    public async Task VirtualDisksNeedAdminAndSkipBlockedDisks()
    {
        var engine = new FakeEngine();
        engine.Replies["vdisk.scan"] = new VirtualDiskList
        {
            Disks = [new() { Id = 1, Path = "/w/ext4.vhdx", OnDiskBytes = 10 }, new() { Id = 2, Path = "/d/sparse.vhdx", Blocked = "sparse: Windows already shrinks it" }],
        };
        engine.Replies["vdisk.run"] = new VirtualDiskResult { Results = [new() { Path = "/w/ext4.vhdx", Status = "compacted", FreedBytes = 4 }], Freed = 4 };
        var host = new FakeHost();
        var vm = new VirtualDisksViewModel(engine, host);
        await vm.ScanCommand.ExecuteAsync(null);
        Assert.IsFalse(vm.Disks[1].IsSelected || vm.Disks[1].CanSelect, "a blocked disk can be chosen");
        Assert.IsFalse(vm.CompactCommand.CanExecute(null), "compacting offered without administrator rights");

        engine.Replies["vdisk.scan"] = new VirtualDiskList { Admin = true, Disks = ((VirtualDiskList)engine.Replies["vdisk.scan"]).Disks };
        await vm.ScanCommand.ExecuteAsync(null);
        await vm.CompactCommand.ExecuteAsync(null);
        StringAssert.Contains(host.LastMessage, "wsl --shutdown");
        StringAssert.Contains(host.LastMessage, "unsaved work");
        Assert.AreEqual("""{"ids":[1]}""", engine.Sent("vdisk.run"));
        Assert.AreEqual("Returned 4 B", vm.Disks[0].Outcome);
    }

    [TestMethod]
    public async Task OptimizeKeepsTheComponentStoreOptInAndPreviewsWithoutAsking()
    {
        var engine = new FakeEngine();
        engine.Replies["optimize.list"] = new OptimizeList
        {
            Tasks = [new() { Id = "dns", Name = "DNS" }, new() { Id = "ssd_trim", Name = "TRIM", AdminRequired = true }, new() { Id = "component_store", Name = "WinSxS", AdminRequired = true }],
        };
        engine.Replies["optimize.run"] = new OptimizeResult { Tasks = [new() { Id = "dns", Status = "skipped" }] };
        var host = new FakeHost { Confirm = false };
        var vm = new OptimizeViewModel(engine, host);
        await vm.LoadCommand.ExecuteAsync(null);
        Assert.IsTrue(vm.NeedsAdmin);
        CollectionAssert.AreEqual(new[] { "dns" }, vm.Tasks.Selected.Select(r => r.Task.Id).ToArray(), "admin-only tasks and --deep start unselected");
        Assert.IsFalse(vm.Tasks[1].CanSelect);

        await vm.PreviewCommand.ExecuteAsync(null);
        Assert.AreEqual(0, host.Confirmations, "a preview changes nothing, so it does not ask");
        Assert.AreEqual("""{"ids":["dns"],"dry_run":true}""", engine.Sent("optimize.run"));

        engine.Calls.Clear();
        await vm.OptimizeCommand.ExecuteAsync(null);
        Assert.IsFalse(engine.Asked("optimize.run"), "declining still optimized");
    }

    [TestMethod]
    public async Task ScheduleSendsTheCliFormatsAndAsksFirst()
    {
        var engine = new FakeEngine();
        engine.Replies["schedule.get"] = new ScheduleInfo
        {
            Safe = [new() { Id = "temp", Name = "Temporary files" }],
            OptIn = [new() { Id = "npm", Name = "npm cache" }, new() { Id = "docker", Name = "Docker" }],
        };
        engine.Replies["schedule.set"] = new ScheduleStatus { Enabled = true, DryRun = true, WouldClean = [new() { Id = "temp", Freed = 9 }] };
        var host = new FakeHost { Confirm = false };
        var vm = new ScheduleViewModel(engine, host);
        await vm.LoadCommand.ExecuteAsync(null);
        vm.Every = "daily";
        vm.At = new TimeSpan(7, 5, 0);
        vm.LowSpacePercent = 0;
        vm.OptIn[0].IsSelected = true;

        await vm.PreviewCommand.ExecuteAsync(null);
        Assert.AreEqual("""{"every":"daily","at":"07:05","low_space":"off","add":["npm"],"dry_run":true}""", engine.Sent("schedule.set"));
        Assert.AreEqual("Temporary files", vm.WouldClean.Single().Label);

        engine.Calls.Clear();
        await vm.TurnOnCommand.ExecuteAsync(null);
        Assert.IsFalse(engine.Asked("schedule.set"), "declining still registered the task");
        StringAssert.Contains(host.LastMessage, "without asking");
        await vm.TurnOffCommand.ExecuteAsync(null);
        Assert.IsFalse(engine.Asked("schedule.off"));
    }

    [TestMethod]
    public async Task StartupRemovesOnlyDisabledEntriesAfterConfirming()
    {
        var engine = new FakeEngine();
        engine.Replies["startup.list"] = new StartupList
        {
            Entries = [new() { Id = 1, Name = "On", Enabled = true }, new() { Id = 2, Name = "Off" }, new() { Id = 3, Name = "Machine", AdminRequired = true }],
        };
        engine.Replies["startup.remove"] = new StartupRemoveResult { Removed = 1 };
        var host = new FakeHost { Confirm = false };
        var vm = new SystemViewModel(engine, host);
        await vm.LoadStartupCommand.ExecuteAsync(null);
        Assert.IsTrue(vm.NeedsAdmin);

        await vm.ToggleCommand.ExecuteAsync(vm.Startup[2]);
        Assert.IsFalse(engine.Asked("startup.toggle"), "a machine-wide entry was changed without administrator rights");

        await vm.RemoveDisabledCommand.ExecuteAsync(null);
        Assert.IsFalse(engine.Asked("startup.remove"), "declining still removed");
        host.Confirm = true;
        await vm.RemoveDisabledCommand.ExecuteAsync(null);
        Assert.AreEqual("""{"ids":[2]}""", engine.Sent("startup.remove"), "only the disabled entry this user can change");
        StringAssert.Contains(host.LastMessage, "can't be undone");
    }

    [TestMethod]
    public async Task DiagnosticsOfferElevationForThePrivilegeWarningAndCopyAReport()
    {
        var engine = new FakeEngine();
        engine.Replies["doctor.run"] = new DoctorSnapshot
        {
            Passed = 1, Warnings = 1,
            Results = [new() { Id = "privilege", Name = "Privileges", Status = "WARN", Message = "Standard user" }, new() { Id = "temp_writable", Name = "Temp", Status = "PASS", Message = "ok" }],
        };
        var host = new FakeHost();
        var vm = new DiagnosticsViewModel(engine, host);
        await vm.RunHealthCheckCommand.ExecuteAsync(null);
        Assert.IsTrue(vm.CanElevate);
        Assert.AreEqual("1 passed, 1 warning, 0 failed.", vm.HealthSummary);
        vm.CopyReportCommand.Execute(null);
        StringAssert.Contains(host.Copied, "[WARN] Privileges: Standard user");
    }

    [TestMethod]
    public async Task SettingsRemovesOnlyAfterConfirmingThenExits()
    {
        var engine = new FakeEngine();
        engine.Replies["remove.plan"] = new RemovePlan { Exe = "/d/du", DataDir = "/data", KeptBytes = 2048, SetupInstalled = true };
        engine.Replies["remove.run"] = new Empty();
        engine.Replies["update.install"] = new UpdateInstallResult { Version = "9.0.0" };
        var host = new FakeHost { Confirm = false };
        var vm = new SettingsViewModel(engine, host, "1.4.0");
        await vm.LoadCommand.ExecuteAsync(null);
        StringAssert.Contains(vm.RemoveText, "Settings > Apps");

        await vm.RemoveCommand.ExecuteAsync(null);
        Assert.IsFalse(engine.Asked("remove.run") || host.Exited, "declining still removed Duster");
        host.Confirm = true;
        await vm.RemoveCommand.ExecuteAsync(null);
        StringAssert.Contains(host.LastMessage, "2 KB");
        Assert.IsTrue(engine.Asked("remove.run") && host.Exited);

        await vm.ReinstallCommand.ExecuteAsync(null);
        Assert.AreEqual("""{"force":true}""", engine.Sent("update.install"));
        Assert.IsTrue(vm.RestartNeeded);
        vm.RestartCommand.Execute(null);
        Assert.IsTrue(host.Restarted);
    }

    [TestMethod]
    public async Task ProtectedInstallCannotRemoveItself()
    {
        var engine = new FakeEngine();
        engine.Replies["remove.plan"] = new RemovePlan { Protected = true };
        var host = new FakeHost();
        var vm = new SettingsViewModel(engine, host, "1.4.0");
        await vm.LoadCommand.ExecuteAsync(null);
        await vm.RemoveCommand.ExecuteAsync(null);
        Assert.AreEqual(0, host.Confirmations);
        Assert.IsFalse(engine.Asked("remove.run"));
    }

    [TestMethod]
    public async Task HomeLivePollsOnlyWhileVisible()
    {
        var engine = new FakeEngine { Connected = true };
        engine.Replies["status.get"] = new SystemStats { HostName = "pc", NetDownSec = 2048, HealthScore = 90, UptimeSeconds = 3700 };
        var vm = new HomeViewModel(engine, new FakeHost(), _ => { }) { LiveInterval = TimeSpan.FromMilliseconds(10) };
        await Task.Delay(50);
        var initial = engine.StatusCalls; // the one refresh on connect
        vm.IsLive = true;
        await Task.Delay(100);
        Assert.AreEqual(initial, engine.StatusCalls, "Live polled a page nobody was looking at");

        vm.IsVisible = true;
        await Task.Delay(150);
        Assert.IsGreaterThan(initial + 1, engine.StatusCalls);
        vm.IsVisible = false;
        await Task.Delay(30);
        var after = engine.StatusCalls;
        await Task.Delay(100);
        Assert.AreEqual(after, engine.StatusCalls, "Live kept polling after leaving Home");

        Assert.AreEqual("down 2 KB/s, up 0 B/s", vm.Details.Single(d => d.Label == "Network").Value);
        Assert.AreEqual("1 h 1 min", vm.Details.Single(d => d.Label == "Up for").Value);
    }

    [TestMethod]
    public async Task AnalyzePassesHistoryOptionsAndOpensAChangeWithItsBreadcrumb()
    {
        var root = new AnalyzeFolder { Id = 1, Path = "/r" };
        var engine = new FakeEngine
        {
            Analysis = new() { Root = root, Changes = new() { Entries = [new() { Id = 7, Path = "/r/a/b/new.iso", Delta = 5 }, new() { Path = "/gone", Delta = -1 }] } },
            Folders = { [7] = new AnalyzeFolder { Id = 7, Path = "/r/a/b", Trail = [new() { Id = 1, Path = "/r" }, new() { Id = 5, Path = "/r/a" }] } },
        };
        engine.Replies["analyze.reveal"] = new Empty();
        var vm = new AnalyzeViewModel(engine, new FakeHost()) { Path = "/r", Since = "7d" };
        await vm.ScanCommand.ExecuteAsync(null);
        Assert.AreEqual(("7d", false), engine.AnalyzeOptions);

        await vm.OpenChangeCommand.ExecuteAsync(vm.Changes[0]);
        CollectionAssert.AreEqual(new[] { "/r", "/r/a", "/r/a/b" }, vm.Trail.Select(f => f.Path).ToArray());
        Assert.IsFalse(vm.Changes[1].CanOpen);

        await vm.RevealChangeCommand.ExecuteAsync(vm.Changes[1]);
        Assert.AreEqual("""{"id":0,"change":2}""", engine.Sent("analyze.reveal"));

        vm.Since = AnalyzeViewModel.PreviousScan;
        vm.NoHistory = true;
        await vm.ScanCommand.ExecuteAsync(null);
        Assert.AreEqual(("", true), engine.AnalyzeOptions);
        StringAssert.Contains(vm.ChangesSummary, "History was off");
    }

    [TestMethod]
    public async Task RestorePreviewMovesNothingAndShowsTheExpiryNote()
    {
        var engine = new FakeEngine { Connected = true };
        engine.Replies["restore.list"] = new RestoreSessionList
        {
            Sessions = [new() { Id = "s1", Items = [new() { Number = 1 }] }],
            Expired = "Removed 1 kept session past its 7 days.",
        };
        engine.Replies["restore.run"] = new RestoreRunResult { Results = [new() { Status = "would restore", Size = 3 }] };
        engine.Replies["oplog.list"] = new OplogList { Entries = [new() { Command = "purge", Action = "quarantine" }] };
        var vm = new RestoreViewModel(engine, new FakeHost());
        await vm.RefreshCommand.ExecuteAsync(null);
        StringAssert.Contains(vm.Status, "past its 7 days");

        await vm.PreviewSessionCommand.ExecuteAsync(null);
        Assert.AreEqual("""{"id":"s1","item":0,"dry_run":true}""", engine.Sent("restore.run"));
        StringAssert.StartsWith(vm.Status, "Preview: would restore 1 item (3 B)");

        await vm.LoadActivityCommand.ExecuteAsync(null);
        Assert.AreEqual("purge", vm.Activity.Single().Command);
    }
}
