using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace Duster.Core.ViewModels;

/// <summary>
/// What every tool page shares: one operation at a time, a status line, Stop,
/// and engine failures shown as text, never thrown into the UI.
/// </summary>
public abstract partial class ToolViewModel(IEngineClient engine, IAppHost host) : ObservableObject
{
    private CancellationTokenSource? _cts;

    protected IEngineClient Engine { get; } = engine;
    protected IAppHost Host { get; } = host;

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(IsIdle))]
    public partial bool IsBusy { get; set; }

    public bool IsIdle => !IsBusy;

    [ObservableProperty]
    public partial string Status { get; set; } = "";

    /// <summary>Relaunches Duster elevated (UAC); the engine inherits the rights.</summary>
    [RelayCommand]
    private void RestartAsAdministrator()
    {
        if (!Host.RestartAsAdministrator())
        {
            Status = "Administrator restart was canceled.";
        }
    }

    /// <summary>Asks the engine to stop the running operation at its next safe point.</summary>
    [RelayCommand]
    private void Stop() => _cts?.Cancel();

    /// <summary>Runs one engine operation; a second one while busy is ignored.</summary>
    protected async Task RunAsync(Func<CancellationToken, Task> action, string canceled = "Stopped.")
    {
        if (IsBusy)
        {
            return;
        }
        using var cts = new CancellationTokenSource();
        _cts = cts;
        IsBusy = true;
        try
        {
            await action(cts.Token);
        }
        catch (EngineException ex) when (ex.Kind == EngineErrorKind.Canceled)
        {
            Status = canceled;
        }
        catch (OperationCanceledException)
        {
            Status = canceled;
        }
        catch (EngineException ex)
        {
            Status = ex.Message;
        }
        finally
        {
            _cts = null;
            IsBusy = false;
        }
    }

    /// <summary>Asks first; true when the user confirmed.</summary>
    protected Task<bool> ConfirmAsync(string title, string message, string confirm) => Host.ConfirmAsync(title, message, confirm);
}
