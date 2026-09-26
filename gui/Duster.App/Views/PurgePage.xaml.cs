using Duster.Core.ViewModels;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace Duster.App.Views;

public sealed partial class PurgePage : Page
{
    public PurgePage()
    {
        InitializeComponent();
    }

    public PurgeViewModel ViewModel { get; private set; } = null!;

    // MainWindow passes the page's long-lived ViewModel as the navigation parameter.
    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        ViewModel = (PurgeViewModel)e.Parameter;
        Bindings.Update();
        base.OnNavigatedTo(e);
    }
}
