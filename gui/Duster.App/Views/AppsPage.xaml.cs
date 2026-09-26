using Duster.Core.ViewModels;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace Duster.App.Views;

public sealed partial class AppsPage : Page
{
    public AppsPage()
    {
        InitializeComponent();
    }

    public AppsViewModel ViewModel { get; private set; } = null!;

    // MainWindow passes the page's long-lived ViewModel as the navigation parameter.
    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        ViewModel = (AppsViewModel)e.Parameter;
        Bindings.Update();
        base.OnNavigatedTo(e);
        if (!ViewModel.Loaded)
        {
            ViewModel.LoadCommand.Execute(null);
        }
    }

    // SelectedItem is object; x:Bind cannot two-way it into a typed property without a converter.
    private void AppList_SelectionChanged(object sender, SelectionChangedEventArgs e) =>
        ViewModel.SelectedApp = AppList.SelectedItem as AppRow;
}
