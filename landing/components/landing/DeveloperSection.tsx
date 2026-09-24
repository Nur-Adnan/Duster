import { buttonVariants } from "@heroui/react/button";
import { developers, links } from "@/lib/content";
import { AutomationFlow } from "./automation/AutomationFlow";
import { DeveloperGlyph } from "./automation/glyphs";
import { HelpOutput } from "./automation/HelpOutput";
import { Container } from "./Container";
import { WithFlags } from "./ui/Flags";
import { ExternalArrow } from "./ui/icons";
import { Section, SectionHeader } from "./ui/Section";

// Real output captured from the actual binary. Do not reformat or edit.
const CTAS = [
  { href: links.docs, label: developers.ctaPrimary, variant: "primary" },
  { href: links.repo, label: developers.ctaSecondary, variant: "outline" },
] as const;

const REAL_HELP_OUTPUT = `  C:\\>du

      /   ______   _    _   _____  _______  ______  _____   │
     /   |  __  \\ | |  | | / ____|__   __| |  ____||  __ \\  │ System Maintenance CLI v1.2.0
    /    | |  \\  \\| |  | || (___    | |    | |__   | |__) | │ Keep your system clean. Keep it running smooth.
   /_    | |  |  || |  | | \\___ \\   | |    |  __|  |  _  /  │
  \\--/   | |__/  /| |__| | ____) |  | |    | |____ | | \\ \\  │
  /__/   |______/  \\____/ |_____/   |_|    |______||_|  \\_\\ │

  ────────────────────────────────────────────────────────────────────────────────

  DESCRIPTION
    Duster (du) is a powerful, Windows-native command-line cleaning
    and optimization utility designed to reclaim disk space, uninstall software remnants,
    and improve overall system responsiveness.

  USAGE
    du [flags]

  AVAILABLE COMMANDS
    ▪ analyze      Interactive TUI disk space explorer and visual analyzer
    ▪ benchmark    Benchmark scan speed, delete speed, memory, goroutines, and JSON engine
    ▪ clean        Deep clean system temp files, prefetch, browser caches, and error logs
    ▪ completion   Generate the autocompletion script for the specified shell
    ▪ doctor       Diagnose Windows environment, privilege, permission, and terminal compatibility
    ▪ installer    Find and remove large installer files
    ▪ optimize     Optimize PC performance (flush DNS, SSD trim, clean caches)
    ▪ purge        Find and clean developer build artifacts recursively to reclaim space
    ▪ remove       Uninstall Duster and delete all configuration files and logs
    ▪ status       Real-time system health dashboard and live resource monitor
    ▪ uninstall    Interactive application uninstaller and remnants clean sweeper
    ▪ update       Check and update Duster to the latest version
    ▪ vdisk        Shrink WSL and Docker virtual disks
    ▪ verify       Audit protected paths, symlink guards, dry-runs, registry safety, and self-integrity

  FLAGS
    ○ -h, --help      help for du
    ○ -v, --version   version for du

  Use du [command] --help for more information about a command.`;

export function DeveloperSection() {
  return (
    <Section id="developers" tone="base" labelledBy="developers-title">
      <SectionHeader
        id="developers-title"
        eyebrow={developers.eyebrow}
        title={developers.title}
        subhead={developers.body}
        action={
          <div className="flex flex-wrap gap-3 max-sm:flex-col">
            {CTAS.map((cta) => (
              <a
                key={cta.href}
                href={cta.href}
                target="_blank"
                rel="noreferrer"
                className={`${buttonVariants({ variant: cta.variant, size: "lg" })} group min-h-11 rounded-md px-6 max-sm:w-full`}
              >
                {cta.label}
                <ExternalArrow />
                <span className="sr-only"> (opens in new tab)</span>
              </a>
            ))}
          </div>
        }
      />

      <Container className="mt-14 sm:mt-16">
        <AutomationFlow />
      </Container>

      <Container className="mt-10 grid items-start gap-10 lg:mt-12 lg:grid-cols-12 lg:gap-12">
        <ul className="divide-y divide-hairline border-y border-hairline lg:col-span-4">
          {developers.list.map((item) => (
            <li key={item} className="flex items-center gap-4 py-4">
              <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-hairline bg-bg-raised text-accent-text">
                <DeveloperGlyph item={item} />
              </span>
              <span className="text-sm leading-relaxed text-ink">
                <WithFlags>{item}</WithFlags>
              </span>
            </li>
          ))}
        </ul>

        <figure className="min-w-0 lg:col-span-8">
          <HelpOutput text={REAL_HELP_OUTPUT} />
          <figcaption className="mt-3 flex items-center gap-2 text-sm text-ink-faint">
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
              <circle cx="7" cy="7" r="5.75" stroke="currentColor" strokeWidth="1.5" />
              <path d="M7 6.5V10M7 4.25h.01" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
            </svg>
            Real output, captured from the actual binary.
          </figcaption>
        </figure>
      </Container>
    </Section>
  );
}
