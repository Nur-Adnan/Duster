using Duster.App.ViewModels;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace Duster.App.Views;

public sealed partial class AnalyzePage : Page
{
    public AnalyzePage()
    {
        InitializeComponent();
    }

    public AnalyzeViewModel ViewModel { get; private set; } = null!;

    // MainWindow passes the page's long-lived ViewModel as the navigation parameter.
    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        ViewModel = (AnalyzeViewModel)e.Parameter;
        Bindings.Update();
        base.OnNavigatedTo(e);
    }
}
