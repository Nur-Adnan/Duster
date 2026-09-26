using Duster.Core;
using Duster.Core.ViewModels;
using Microsoft.UI.Xaml;
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

    private void EntryList_ItemClick(object sender, ItemClickEventArgs e)
    {
        if (e.ClickedItem is AnalyzeRow row)
        {
            ViewModel.OpenCommand.Execute(row);
        }
    }

    private void Trail_ItemClicked(BreadcrumbBar sender, BreadcrumbBarItemClickedEventArgs args)
    {
        if (args.Item is AnalyzeFolder folder)
        {
            ViewModel.GoToCommand.Execute(folder);
        }
    }

    // SelectedItem is object; x:Bind cannot two-way it into a typed property without a converter.
    private void Rows_SelectionChanged(object sender, SelectionChangedEventArgs e) =>
        ViewModel.SelectedRow = (sender as ListView)?.SelectedItem as AnalyzeRow;

    // Which list is showing is view state only.
    private void Views_SelectionChanged(SelectorBar sender, SelectorBarSelectionChangedEventArgs args)
    {
        FolderPanel.Visibility = Ui.Visible(sender.SelectedItem == FolderView);
        LargestList.Visibility = Ui.Visible(sender.SelectedItem == LargestView);
        ChangesPanel.Visibility = Ui.Visible(sender.SelectedItem == ChangesView);
        EntryList.SelectedItem = null;
        LargestList.SelectedItem = null;
    }
}
