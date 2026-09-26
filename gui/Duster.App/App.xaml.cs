using System.Diagnostics;
using Duster.Core.ViewModels;
using Duster.Infrastructure;
using Microsoft.UI.Xaml;

namespace Duster.App;

public partial class App : Application
{
    private MainWindow? _window;

    public App()
    {
        InitializeComponent();
    }

    protected override void OnLaunched(Microsoft.UI.Xaml.LaunchActivatedEventArgs args)
    {
        // ponytail: diagnostics go to the debugger (winapp run --debug-output shows them);
        // add a log file when there is a support workflow that needs one.
        var engine = new EngineClient(AppContext.BaseDirectory, EnginePath.FileName, message => Debug.WriteLine(message));
        _window = new MainWindow(new ShellViewModel(engine));
        _window.Closed += (_, _) =>
        {
            // Closing stdin is enough: the engine cancels, stops at a safe point and
            // exits, even if this process is gone before DisposeAsync finishes.
            _ = engine.DisposeAsync().AsTask();
        };
        _window.Activate();
    }
}
