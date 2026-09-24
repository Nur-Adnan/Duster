// Automatically resolves site URL:
// 1. NEXT_PUBLIC_SITE_URL if configured (e.g. custom domain or production URL)
// 2. VERCEL_PROJECT_PRODUCTION_URL (automatically provided by Vercel for production)
// 3. VERCEL_URL (automatically provided by Vercel for preview deployments)
// 4. http://localhost:3000 for local development
function getSiteUrl(): URL {
  const envUrl =
    process.env.NEXT_PUBLIC_SITE_URL ||
    (process.env.VERCEL_PROJECT_PRODUCTION_URL ? `https://${process.env.VERCEL_PROJECT_PRODUCTION_URL}` : undefined) ||
    (process.env.VERCEL_URL ? `https://${process.env.VERCEL_URL}` : undefined) ||
    "http://localhost:3000";

  return new URL(envUrl.startsWith("http") ? envUrl : `https://${envUrl}`);
}

export const siteUrl = getSiteUrl();

const REPO = "https://github.com/Nur-Adnan/Duster";

/** Real destinations (all verified to exist in the repo). */
export const links = {
  repo: REPO,
  releases: `${REPO}/releases`,
  latestRelease: `${REPO}/releases/latest`,
  docs: `${REPO}#readme`,
  categories: `${REPO}#cleanup-categories`,
  issues: `${REPO}/issues`,
  newIssue: `${REPO}/issues/new`,
  changelog: `${REPO}/blob/main/CHANGELOG.md`,
  license: `${REPO}/blob/main/LICENSE`,
  security: `${REPO}/blob/main/SECURITY.md`,
  contributing: `${REPO}/blob/main/CONTRIBUTING.md`,
  codeOfConduct: `${REPO}/blob/main/CODE_OF_CONDUCT.md`,
};

export const nav = {
  links: [
    { label: "Commands", href: "#features" },
    { label: "Inside a scan", href: "#spotlight" },
    { label: "Deployments", href: "#usage" },
    { label: "Developers", href: "#developers" },
  ],
  cta: "Get started",
};

export const hero = {
  eyebrow: "One binary for Windows 10 and 11, no dependencies",
  headline: ["Your drive,", "swept clean."],
  subhead:
    "A single-binary Windows CLI that cleans caches, purges dev artifacts, and shows what's actually eating your disk. Every destructive command previews first.",
  install: {
    label: "Install with PowerShell",
    command: "irm https://raw.githubusercontent.com/Nur-Adnan/Duster/main/scripts/install.ps1 | iex",
  },
  ctaPrimary: "Install Duster",
  ctaSecondary: "View source",
  previewStates: [
    { label: "Scanning", detail: "34 categories, 12.4 GB found" },
    { label: "Reviewing", detail: "Select categories to clean" },
    { label: "Cleaned", detail: "12.4 GB reclaimed in 6.2 s" },
  ],
};

export const features = {
  eyebrow: "Commands",
  title: "One tool, thirteen jobs",
  subhead:
    "Five of the thirteen. Run du --help for the rest.",
  cards: [
    {
      title: "clean",
      description: "Sweeps 34 cache categories across the OS, browsers, and dev tools.",
    },
    {
      title: "analyze",
      description: "Drills into any folder to find what's actually eating disk space.",
    },
    {
      title: "purge",
      description: "Hunts down stray node_modules, target, and .gradle folders.",
    },
    {
      title: "doctor",
      description: "Runs ten health checks and tells you exactly what's misconfigured.",
    },
    {
      title: "status",
      description: "A live dashboard for CPU, RAM, disk, network, and thermals.",
    },
  ],
};

export const spotlight = {
  eyebrow: "Inside a scan",
  title: "Every delete is checked before it happens",
  body:
    "Duster resolves every path through the same safety layer: symlinks are never followed, OneDrive placeholders are skipped during cleaning instead of forced online, and protected system folders are refused outright.",
  supporting: [
    {
      title: "Preview before delete",
      body: "Every destructive command takes --dry-run. Nothing is removed without --yes or an explicit confirm.",
    },
    {
      title: "Logged, not silent",
      body: "Every deletion is recorded in a local operations log with what it freed, so you can audit what happened.",
    },
  ],
};

export const showcase = {
  eyebrow: "Reclaimed space",
  title: "What a first run tends to find",
  subhead: "Numbers from real machines that hadn't been cleaned in a while.",
  linkText: "See all 34 categories",
  tiles: [
    { label: "Browser caches", value: "3.1 GB" },
    { label: "npm + pnpm store", value: "2.8 GB" },
    { label: "Windows Update cache", value: "1.9 GB" },
    { label: "GPU shader cache", value: "640 MB" },
    { label: "Crash dumps", value: "410 MB" },
    { label: "Docker artifacts", value: "1.2 GB" },
    { label: "Old installers", value: "980 MB" },
    { label: "Thumbnail cache", value: "310 MB" },
  ],
};

export const flexibleUsage = {
  eyebrow: "Deployments",
  title: "Runs on your terms",
  subhead: "A one-off PowerShell install or a pinned binary in CI. Same tool, same safety rules.",
  columns: [
    {
      label: "Interactive",
      description: "Launch the TUI and step through categories, drivers, and startup apps from the keyboard.",
      points: [
        "Bubble Tea terminal UI",
        "Live system dashboard",
        "Category checklist with previews",
        "Keyboard-only navigation",
      ],
    },
    {
      label: "Scripted",
      description: "Pipe it into CI, a scheduled task, or your own tooling.",
      points: [
        "--json output for scripts and CI",
        "--yes for non-interactive runs",
        "Headless when output is piped",
      ],
    },
  ],
};

export const developers = {
  eyebrow: "Automation",
  title: "Built to be scripted",
  body:
    "Most commands emit structured --json, so you can wire Duster into a scheduled task or a CI health check without guessing at its behavior.",
  list: [
    "Structured --json output",
    "Deterministic exit codes",
    "Works headless over SSH or RDP",
  ],
  ctaPrimary: "Read the docs",
  ctaSecondary: "View on GitHub",
};

export const enterprise = {
  eyebrow: "At scale",
  title: "One executable, every workstation",
  subhead: "No installer service, no telemetry endpoint, no dependency to patch across a fleet.",
  nodes: [
    "Deploy",
    "Schedule",
    "Verify",
    "Report",
    "Update",
  ],
  cards: [
    { title: "No runtime", body: "CGO disabled, statically linked. Copy the binary and run it." },
    { title: "Verified releases", body: "SHA-256 checksums and build-provenance attestations on every release." },
    { title: "Auditable", body: "Every clean, purge and uninstall is recorded, with its outcome, in a local operations log." },
    { title: "Self-updating", body: "Checks GitHub releases and rolls back if swapping the binary fails." },
  ],
};

export const integrations = {
  eyebrow: "Release pipeline",
  title: "Shipped the way it's built",
  subhead: "Every change runs these gates, and the Windows smoke and end-to-end tests run before each release.",
  statusRows: [
    { label: "gofmt, go vet, staticcheck", status: "done" as const },
    { label: "Windows cross-compile", status: "done" as const },
    { label: "Checksums + provenance attestation", status: "done" as const },
    { label: "Authenticode signing (SignPath)", status: "configuring" as const },
  ],
  tiles: ["CI", "Vet", "Fmt", "Sign", "Hash", "Tag", "Smoke", "E2E"],
};

export const faq = {
  title: "Questions worth answering upfront",
  items: [
    {
      q: "Does Duster need to run as administrator?",
      a: "No. Most commands run as a normal user. A handful of operations (prefetch cleanup, drive optimization, DISM component-store cleanup) need elevation, and Duster will tell you when it's blocked instead of failing silently.",
    },
    {
      q: "Will it delete something I still need?",
      a: "Every deletion target is checked against a path-safety layer first: no symlinks, no protected system directories, no forced OneDrive downloads. Destructive commands also support --dry-run so you can preview exactly what would be removed.",
    },
    {
      q: "What happens to Windows.old or the hibernation file?",
      a: "Duster measures and reports them but never deletes them. Only Windows' own cleanup tools are allowed to remove Windows.old, and hibernation is a power setting Duster won't touch.",
    },
    {
      q: "Can I use it in a script or scheduled task?",
      a: "Yes. clean, purge, optimize and vdisk take --yes for unattended runs, and most commands can emit structured --json.",
    },
    {
      q: "Does it phone home?",
      a: "The only network call Duster makes on its own is checking GitHub for a newer release, and that only happens when you run du update.",
    },
    {
      q: "What platforms does it support?",
      a: "Windows 10 and 11 are the only platforms Duster ships a binary for. It cross-compiles cleanly on macOS and Linux so its test suite can run anywhere, but there's no macOS or Linux release.",
    },
  ],
  calloutText: "Still have questions?",
  calloutLink: "Open an issue on GitHub",
};

export const footer = {
  tagline: "A Windows-native deep cleaner, built to be trusted with delete.",
  columns: [
    {
      title: "Product",
      links: [
        { label: "Commands", href: "#features" },
        { label: "Changelog", href: links.changelog },
        { label: "Releases", href: links.releases },
        { label: "Roadmap", href: links.issues },
      ],
    },
    {
      title: "Developers",
      links: [
        { label: "Documentation", href: links.docs },
        { label: "GitHub", href: links.repo },
        { label: "CLI reference", href: links.docs },
        { label: "JSON output", href: "#developers" },
      ],
    },
    {
      title: "Company",
      links: [
        { label: "About", href: links.docs },
        { label: "Security policy", href: links.security },
        { label: "Contributing", href: links.contributing },
      ],
    },
    {
      title: "Legal",
      links: [
        { label: "MIT license", href: links.license },
        { label: "Code of conduct", href: links.codeOfConduct },
      ],
    },
  ],
  copyright: `© ${new Date().getFullYear()} Duster. MIT licensed.`,
};
