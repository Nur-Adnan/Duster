using Duster.Core.ViewModels;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace Duster.App.Views;

public sealed partial class VirtualDisksPage : Page
{
    public VirtualDisksPage()
    {
        InitializeComponent();
    }

    public VirtualDisksViewModel ViewModel { get; private set; } = null!;

    // MainWindow passes the page's long-lived ViewModel as the navigation parameter.
    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        ViewModel = (VirtualDisksViewModel)e.Parameter;
        Bindings.Update();
        base.OnNavigatedTo(e);
        if (!ViewModel.Scanned)
        {
            ViewModel.ScanCommand.Execute(null); // read-only discovery
        }
    }
}
