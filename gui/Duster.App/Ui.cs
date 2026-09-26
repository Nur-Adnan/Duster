using Duster.Core.ViewModels;
using Duster.Core;
using Microsoft.UI.Xaml;

namespace Duster.App;

/// <summary>x:Bind functions (cheaper and simpler than IValueConverter).</summary>
public static class Ui
{
    public static Visibility Visible(bool value) => value ? Visibility.Visible : Visibility.Collapsed;

    public static Visibility Collapsed(bool value) => value ? Visibility.Collapsed : Visibility.Visible;

    public static Visibility VisibleIfText(string? value) => string.IsNullOrEmpty(value) ? Visibility.Collapsed : Visibility.Visible;

    public static bool HasText(string? value) => !string.IsNullOrEmpty(value);

    public static string Bytes(long bytes) => Format.Bytes(bytes);

    public static string SessionSummary(DateTimeOffset created, int items, long size, bool damaged) =>
        $"{created.LocalDateTime:g} · {items} item{(items == 1 ? "" : "s")} · {Format.Bytes(size)}{(damaged ? " · partly unreadable" : "")}";

    public static string RestoreName(string path) => "Restore " + path;

    // Segoe Fluent Icons: Folder, Page.
    public static string ItemGlyph(bool isDir) => isDir ? "\uE8B7" : "\uE8A5";

    // Segoe Fluent Icons: CheckMark, Error, Sync.
    public static string EngineGlyph(EngineState state) => state switch
    {
        EngineState.Connected => "",
        EngineState.Faulted => "",
        _ => "",
    };
}
