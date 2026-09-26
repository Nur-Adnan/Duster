using Duster.App.ViewModels;
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
        _pages = new()
        {
            ["home"] = (typeof(HomePage), new HomeViewModel(viewModel.Engine, Navigate)),
            ["clean"] = (typeof(CleanPage), new CleanViewModel(viewModel.Engine)),
            ["restore"] = (typeof(RestorePage), new RestoreViewModel()),
            ["analyze"] = (typeof(AnalyzePage), new AnalyzeViewModel()),
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
        NavView.SelectedItem = NavView.MenuItems.OfType<NavigationViewItem>().First(i => i.Tag is string t && t == tag);

    private void NavView_SelectionChanged(NavigationView sender, NavigationViewSelectionChangedEventArgs args)
    {
        if (args.SelectedItem is NavigationViewItem { Tag: string tag } && _pages.TryGetValue(tag, out var page))
        {
            NavFrame.Navigate(page.Page, page.ViewModel);
        }
    }

    private void TitleBar_PaneToggleRequested(TitleBar sender, object args) => NavView.IsPaneOpen = !NavView.IsPaneOpen;
}
