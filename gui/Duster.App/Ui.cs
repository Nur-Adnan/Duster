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

    // Segoe Fluent Icons: CheckMark, Warning, ErrorBadge, Info. Doctor and verify say
    // PASS/WARN/FAIL/SKIPPED, the security audit secure/warning/critical. The status
    // text is always shown or read out too, so the icon is never the only signal.
    public static string StatusGlyph(string status) => status.ToLowerInvariant() switch
    {
        "pass" or "secure" => "\uE73E",
        "warn" or "warning" => "\uE7BA",
        "fail" or "critical" => "\uEA39",
        _ => "\uE946",
    };

    public static string Signed(bool signed) => signed ? "Signed" : "Unsigned";

    /// <summary>"purge · quarantine": which command did what, for the activity log.</summary>
    public static string Action(string command, string action) => $"{command} · {action}";

    // Segoe Fluent Icons: CheckMark, Error, Sync.
    public static string EngineGlyph(EngineState state) => state switch
    {
        EngineState.Connected => "",
        EngineState.Faulted => "",
        _ => "",
    };
}
