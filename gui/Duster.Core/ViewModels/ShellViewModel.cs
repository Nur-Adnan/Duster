using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Duster.Core;

namespace Duster.Core.ViewModels;

/// <summary>Engine connection state for the window chrome: pane footer and error bar.</summary>
public sealed partial class ShellViewModel : ObservableObject
{
    public ShellViewModel(IEngineClient engine)
    {
        Engine = engine;
        var ui = SynchronizationContext.Current; // constructed on the UI thread
        engine.StateChanged += (_, _) => ui?.Post(_ => ApplyState(), null);
        ApplyState();
    }

    public IEngineClient Engine { get; }

    [ObservableProperty]
    public partial EngineState State { get; set; }

    [ObservableProperty]
    public partial string EngineStatus { get; set; } = "";

    [ObservableProperty]
    public partial bool IsErrorOpen { get; set; }

    [ObservableProperty]
    public partial string ErrorTitle { get; set; } = "";

    [ObservableProperty]
    public partial string ErrorMessage { get; set; } = "";

    [RelayCommand]
    private async Task StartEngineAsync()
    {
        try
        {
            // Process.Start is synchronous; keep it off the UI thread.
            await Task.Run(() => Engine.StartAsync());
        }
        catch (EngineException)
        {
            // Not swallowed: the client is now Faulted and ApplyState shows why.
        }
    }

    private void ApplyState()
    {
        var state = Engine.State;
        var hello = Engine.Hello;
        var fault = Engine.Fault;
        State = state;
        EngineStatus = state switch
        {
            EngineState.Connected when hello is not null => $"Engine {hello.Version}{(hello.Admin ? " · administrator" : "")}",
            EngineState.Starting => "Starting engine…",
            EngineState.Faulted => "Engine not running",
            _ => "Engine stopped",
        };
        if (state == EngineState.Faulted && fault is not null)
        {
            ErrorTitle = Title(fault.Kind);
            ErrorMessage = fault.Message;
            IsErrorOpen = true;
        }
        else if (state == EngineState.Connected)
        {
            IsErrorOpen = false;
        }
    }

    public static string Title(EngineErrorKind kind) => kind switch
    {
        EngineErrorKind.NotFound => "Engine not found",
        EngineErrorKind.StartFailed => "The engine could not start",
        EngineErrorKind.PermissionDenied => "Permission denied",
        EngineErrorKind.HandshakeFailed => "The engine did not respond",
        EngineErrorKind.ProtocolMismatch => "Engine version mismatch",
        EngineErrorKind.Exited => "The engine stopped",
        EngineErrorKind.InvalidMessage => "Unexpected reply from the engine",
        EngineErrorKind.Rejected => "Request rejected",
        EngineErrorKind.Canceled => "Canceled",
        _ => "Something went wrong",
    };
}
