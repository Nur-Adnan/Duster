using System.ComponentModel;
using System.Diagnostics;
using Duster.Core;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.Windows.Storage.Pickers;

namespace Duster.App;

/// <summary>The window-level services ViewModels ask for (Duster.Core.IAppHost).</summary>
internal sealed class WindowsAppHost(Window window) : IAppHost
{
    private const int ErrorCancelled = 1223; // the user said No at the UAC prompt

    public async Task<bool> ConfirmAsync(string title, string message, string confirmLabel)
    {
        var dialog = new ContentDialog
        {
            Title = title,
            Content = new TextBlock { Text = message, TextWrapping = TextWrapping.Wrap },
            PrimaryButtonText = confirmLabel,
            CloseButtonText = "Cancel",
            // Enter must not confirm a destructive action by accident.
            DefaultButton = ContentDialogButton.Close,
            XamlRoot = window.Content.XamlRoot,
        };
        return await dialog.ShowAsync() == ContentDialogResult.Primary;
    }

    public bool RestartAsAdministrator()
    {
        // The GUI's only ShellExecute: UAC elevation needs the "runas" verb. It
        // relaunches this same Duster.exe with no arguments, never a path or
        // argument that came from input.
        var self = Environment.ProcessPath ?? throw new InvalidOperationException("Duster.exe path unknown");
        try
        {
            Process.Start(new ProcessStartInfo(self) { UseShellExecute = true, Verb = "runas" })?.Dispose();
        }
        catch (Win32Exception ex) when (ex.NativeErrorCode == ErrorCancelled)
        {
            return false;
        }
        // Exiting closes the engine's stdin; it stops at a safe point on its own.
        Application.Current.Exit();
        return true;
    }

    public async Task<string?> PickFolderAsync()
    {
        // Microsoft.Windows.Storage.Pickers (Windows App SDK) rather than
        // Windows.Storage.Pickers: it needs no HWND interop and also works
        // when Duster runs elevated.
        var picker = new FolderPicker(window.AppWindow.Id);
        var result = await picker.PickSingleFolderAsync();
        return result?.Path;
    }
}
