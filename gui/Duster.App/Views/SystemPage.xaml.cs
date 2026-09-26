using Duster.Core.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace Duster.App.Views;

public sealed partial class SystemPage : Page
{
    public SystemPage()
    {
        InitializeComponent();
    }

    public SystemViewModel ViewModel { get; private set; } = null!;

    // MainWindow passes the page's long-lived ViewModel as the navigation parameter.
    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        ViewModel = (SystemViewModel)e.Parameter;
        Bindings.Update();
        base.OnNavigatedTo(e);
        if (!ViewModel.StartupLoaded)
        {
            ViewModel.LoadStartupCommand.Execute(null);
        }
    }

    private void Toggle_Click(object sender, RoutedEventArgs e)
    {
        if (sender is FrameworkElement { Tag: StartupRow row })
        {
            ViewModel.ToggleCommand.Execute(row);
        }
    }

    // Which list is showing is view state only.
    private void Views_SelectionChanged(SelectorBar sender, SelectorBarSelectionChangedEventArgs args)
    {
        StartupPanel.Visibility = Ui.Visible(sender.SelectedItem == StartupView);
        SecurityPanel.Visibility = Ui.Visible(sender.SelectedItem == SecurityView);
        DriversPanel.Visibility = Ui.Visible(sender.SelectedItem == DriversView);
    }
}
