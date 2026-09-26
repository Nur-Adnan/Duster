using Duster.Core.ViewModels;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace Duster.App.Views;

public sealed partial class InstallersPage : Page
{
    public InstallersPage()
    {
        InitializeComponent();
    }

    public InstallersViewModel ViewModel { get; private set; } = null!;

    // MainWindow passes the page's long-lived ViewModel as the navigation parameter.
    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        ViewModel = (InstallersViewModel)e.Parameter;
        Bindings.Update();
        base.OnNavigatedTo(e);
        if (ViewModel.Items.Count == 0 && ViewModel.Status.Length == 0)
        {
            ViewModel.ScanCommand.Execute(null); // read-only, top level of Downloads
        }
    }
}
