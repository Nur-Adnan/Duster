using System.Reflection;
using Duster.Core.ViewModels;
using Duster.App.Views;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace Duster.App;

public sealed partial class MainWindow : Window
{
    // Page ViewModels live as long as the window, so a page keeps its state
    // (scan results, loaded stats) when the user navigates away and back.
    private readonly Dictionary<string, (Type Page, object ViewModel)> _pages;

    public MainWindow(ShellViewModel viewModel)
    {
        ViewModel = viewModel;
        var host = new WindowsAppHost(this);
        var engine = viewModel.Engine;
        _pages = new()
        {
            ["home"] = (typeof(HomePage), new HomeViewModel(engine, host, Navigate)),
            ["clean"] = (typeof(CleanPage), new CleanViewModel(engine, host)),
            ["purge"] = (typeof(PurgePage), new PurgeViewModel(engine, host)),
            ["installers"] = (typeof(InstallersPage), new InstallersViewModel(engine, host)),
            ["apps"] = (typeof(AppsPage), new AppsViewModel(engine, host)),
            ["analyze"] = (typeof(AnalyzePage), new AnalyzeViewModel(engine, host)),
            ["vdisk"] = (typeof(VirtualDisksPage), new VirtualDisksViewModel(engine, host)),
            ["restore"] = (typeof(RestorePage), new RestoreViewModel(engine, host)),
            ["optimize"] = (typeof(OptimizePage), new OptimizeViewModel(engine, host)),
            ["system"] = (typeof(SystemPage), new SystemViewModel(engine, host)),
            ["diagnostics"] = (typeof(DiagnosticsPage), new DiagnosticsViewModel(engine, host)),
            ["schedule"] = (typeof(SchedulePage), new ScheduleViewModel(engine, host)),
            ["settings"] = (typeof(SettingsPage), new SettingsViewModel(engine, host, AppVersion())),
        };
        InitializeComponent();

        ExtendsContentIntoTitleBar = true;
        SetTitleBar(AppTitleBar);
        AppWindow.TitleBar.PreferredHeightOption = TitleBarHeightOption.Tall;
        AppWindow.SetIcon("Assets/AppIcon.ico");

        ViewModel.StartEngineCommand.Execute(null);
    }

    public ShellViewModel ViewModel { get; }

    private void Navigate(string tag) =>
        NavView.SelectedItem = NavView.MenuItems.Concat(NavView.FooterMenuItems).OfType<NavigationViewItem>().First(i => i.Tag is string t && t == tag);

    /// <summary>The release version (dotnet publish -p:Version), without the build metadata suffix.</summary>
    private static string AppVersion() =>
        typeof(MainWindow).Assembly.GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion.Split('+')[0] ?? "unknown";

    private void NavView_SelectionChanged(NavigationView sender, NavigationViewSelectionChangedEventArgs args)
    {
        if (args.SelectedItem is NavigationViewItem { Tag: string tag } && _pages.TryGetValue(tag, out var page))
        {
            NavFrame.Navigate(page.Page, page.ViewModel);
        }
    }

    private void TitleBar_PaneToggleRequested(TitleBar sender, object args) => NavView.IsPaneOpen = !NavView.IsPaneOpen;
}
