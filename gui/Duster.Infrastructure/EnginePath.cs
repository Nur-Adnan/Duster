using Duster.Core;

namespace Duster.Infrastructure;

public static class EnginePath
{
    public const string FileName = "du.exe";

    /// <summary>
    /// Returns the engine beside the app. Refuses anything but a regular file:
    /// a link could point the GUI at a binary Duster did not install.
    /// </summary>
    public static string Resolve(string appDirectory, string fileName = FileName)
    {
        var path = Path.GetFullPath(Path.Combine(appDirectory, fileName));
        var info = new FileInfo(path);
        if (info.Exists && info.Attributes.HasFlag(FileAttributes.ReparsePoint))
        {
            throw new EngineException(EngineErrorKind.StartFailed,
                $"{fileName} next to Duster is a link, not the engine Duster installed. Reinstall Duster.");
        }
        if (!info.Exists)
        {
            throw new EngineException(EngineErrorKind.NotFound,
                $"The Duster engine ({fileName}) was not found next to Duster. Reinstall Duster.");
        }
        return path;
    }
}
