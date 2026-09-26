using Duster.Core.ViewModels;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace Duster.App.Views;

public sealed partial class DiagnosticsPage : Page
{
    public DiagnosticsPage()
    {
        InitializeComponent();
    }

    public DiagnosticsViewModel ViewModel { get; private set; } = null!;

    // MainWindow passes the page's long-lived ViewModel as the navigation parameter.
    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        ViewModel = (DiagnosticsViewModel)e.Parameter;
        Bindings.Update();
        base.OnNavigatedTo(e);
    }

    // Which list is showing is view state only.
    private void Views_SelectionChanged(SelectorBar sender, SelectorBarSelectionChangedEventArgs args)
    {
        HealthPanel.Visibility = Ui.Visible(sender.SelectedItem == HealthView);
        SelfTestPanel.Visibility = Ui.Visible(sender.SelectedItem == SelfTestView);
        BenchmarkPanel.Visibility = Ui.Visible(sender.SelectedItem == BenchmarkView);
    }
}
