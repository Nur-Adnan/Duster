namespace Duster.App.ViewModels;

// Restore and Analyze have no engine methods yet (milestones 4 and 5); until
// then their pages point at the CLI command that does the job.

public sealed class RestoreViewModel
{
    public string CliCommand => "du restore";
}

public sealed class AnalyzeViewModel
{
    public string CliCommand => "du analyze";
}
