using Duster.Core;
using Duster.Core.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace Duster.App.Views;

public sealed partial class RestorePage : Page
{
    public RestorePage()
    {
        InitializeComponent();
    }

    public RestoreViewModel ViewModel { get; private set; } = null!;

    // MainWindow passes the page's long-lived ViewModel as the navigation parameter.
    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        ViewModel = (RestoreViewModel)e.Parameter;
        Bindings.Update();
        base.OnNavigatedTo(e);
        ViewModel.RefreshCommand.Execute(null);
    }

    // SelectedItem is object; x:Bind cannot two-way it into a typed property without a converter.
    private void SessionList_SelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        if (SessionList.SelectedItem is RestoreSession session)
        {
            ViewModel.SelectedSession = session;
        }
    }

    private void RestoreItem_Click(object sender, RoutedEventArgs e)
    {
        if (sender is FrameworkElement { Tag: RestoreItem item })
        {
            ViewModel.RestoreItemCommand.Execute(item);
        }
    }
}
