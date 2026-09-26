using Duster.Core.ViewModels;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace Duster.App.Views;

public sealed partial class OptimizePage : Page
{
    public OptimizePage()
    {
        InitializeComponent();
    }

    public OptimizeViewModel ViewModel { get; private set; } = null!;

    // MainWindow passes the page's long-lived ViewModel as the navigation parameter.
    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        ViewModel = (OptimizeViewModel)e.Parameter;
        Bindings.Update();
        base.OnNavigatedTo(e);
        if (!ViewModel.Loaded)
        {
            ViewModel.LoadCommand.Execute(null); // lists tasks and measures; runs nothing
        }
    }
}
