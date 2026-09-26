namespace Duster.Core.ViewModels;

public static class Format
{
    private static readonly string[] Units = ["B", "KB", "MB", "GB", "TB"];

    /// <summary>1024-based, like the CLI's formatBytes: "512 B", "1.5 GB".</summary>
    public static string Bytes(double bytes)
    {
        var unit = 0;
        while (bytes >= 1024 && unit < Units.Length - 1)
        {
            bytes /= 1024;
            unit++;
        }
        return unit == 0 ? $"{bytes:0} B" : $"{bytes:0.#} {Units[unit]}";
    }

    public static string Plural(int n, string one, string many) => $"{n:N0} {(n == 1 ? one : many)}";

    public static string Rate(double bytesPerSecond) => Bytes(bytesPerSecond) + "/s";

    /// <summary>"3 d 4 h", "5 h 12 min", "7 min".</summary>
    public static string Duration(TimeSpan t) =>
        t.TotalDays >= 1 ? $"{(int)t.TotalDays} d {t.Hours} h" : t.TotalHours >= 1 ? $"{(int)t.TotalHours} h {t.Minutes} min" : $"{Math.Max(0, t.Minutes)} min";
}
