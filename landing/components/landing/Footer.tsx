import { footer, links } from "@/lib/content";
import { Container } from "./Container";
import { InstallCommand } from "./hero/InstallCommand";
import { Logo } from "./Logo";
import { ArrowIcon, ExternalArrow, GitHubGlyph } from "./ui/icons";

export function Footer() {
  return (
    <footer className="relative overflow-hidden border-t border-hairline bg-surface-dark text-ink-muted">
      {/* Closing call to action */}
      <Container className="pt-20 pb-16 sm:pt-24">
        <div className="flex flex-col gap-8 border-b border-hairline pb-16 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-xl">
            <h2
              id="footer-cta"
              className="text-balance text-3xl font-semibold tracking-tight text-ink sm:text-4xl"
            >
              Run it once. See what it finds.
            </h2>
            <p className="mt-3 max-w-[60ch] text-pretty leading-relaxed text-ink-muted">
              Nothing is removed until you say so.
            </p>
          </div>

          <div className="flex min-w-0 flex-col gap-4 lg:max-w-140 lg:items-end">
            <div className="w-full min-w-0 lg:w-140">
              <InstallCommand />
            </div>
            <a
              href={links.repo}
              target="_blank"
              rel="noreferrer"
              className="group inline-flex min-h-11 items-center gap-2 self-start rounded-md text-sm font-medium text-ink transition-colors hover:text-accent-text lg:self-end"
            >
              View the source on GitHub
              <span className="sr-only"> (opens in new tab)</span>
              <ArrowIcon className="size-4" />
            </a>
          </div>
        </div>
      </Container>

      {/* Brand and link columns */}
      <Container className="grid gap-12 pb-16 lg:grid-cols-[1.1fr_2fr]">
        <div>
          <Logo />
          <p className="mt-4 max-w-xs text-sm leading-relaxed text-ink-muted">
            {footer.tagline}
          </p>
        </div>

        <nav aria-label="Footer" className="grid grid-cols-2 gap-x-8 gap-y-10 sm:grid-cols-4">
          {footer.columns.map((col) => (
            <div key={col.title}>
              <h3 className="text-sm font-medium text-ink-faint">{col.title}</h3>
              <ul className="mt-3 flex flex-col">
                {col.links.map((link) => {
                  const external = link.href.startsWith("http");
                  return (
                    <li key={link.label}>
                      <a
                        href={link.href}
                        {...(external ? { target: "_blank", rel: "noreferrer" } : {})}
                        className="group inline-flex min-h-11 items-center gap-1.5 text-sm text-ink-muted transition-colors hover:text-ink"
                      >
                        {link.label}
                        {external && (
                          <>
                            <span className="sr-only"> (opens in new tab)</span>
                            <ExternalArrow className="text-ink-faint" />
                          </>
                        )}
                      </a>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </nav>
      </Container>

      {/* Bottom bar */}
      <div className="border-t border-hairline">
        <Container className="flex flex-col items-center gap-3 py-5 text-xs text-ink-faint sm:flex-row sm:justify-between">
          <span className="tabular-nums">{footer.copyright}</span>
          <a
            href={links.repo}
            target="_blank"
            rel="noreferrer"
            aria-label="GitHub (opens in new tab)"
            className="inline-flex h-11 w-11 items-center justify-center rounded-md text-ink-faint transition-colors hover:text-ink focus-visible:bg-bg-raised focus-visible:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent-text"
          >
            <GitHubGlyph className="size-5" />
          </a>
        </Container>
      </div>

      {/* Signature: oversized, low-contrast wordmark clipped at the bottom edge */}
      <div
        aria-hidden="true"
        className="pointer-events-none relative h-[clamp(3.5rem,13vw,12rem)] select-none overflow-hidden"
      >
        <span className="absolute inset-x-0 top-0 block bg-linear-to-b from-white/7.5 to-white/0 bg-clip-text text-center font-sans text-[clamp(6rem,22vw,20rem)] font-semibold leading-[0.8] tracking-tighter whitespace-nowrap text-transparent">
          duster
        </span>
      </div>
    </footer>
  );
}
