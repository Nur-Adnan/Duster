import { buttonVariants } from "@heroui/react/button";
import { hero, links } from "@/lib/content";
import { ExternalArrow, GitHubGlyph } from "../ui/icons";

/*
 * The CTAs navigate, so they are links styled with HeroUI's own button
 * classes (buttonVariants), not <Button>, which renders a <button>. The
 * /button subpath is safe to import from a Server Component.
 */
export function HeroCtas() {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <a
        href={links.latestRelease}
        target="_blank"
        rel="noreferrer"
        className={`${buttonVariants({ variant: "primary", size: "lg" })} min-h-11 rounded-md px-7 max-sm:w-full`}
      >
        {hero.ctaPrimary}
        <span className="sr-only"> (opens in new tab)</span>
      </a>
      <a
        href={links.repo}
        target="_blank"
        rel="noreferrer"
        className={`${buttonVariants({ variant: "outline", size: "lg" })} group min-h-11 rounded-md px-6 max-sm:w-full`}
      >
        <GitHubGlyph />
        {hero.ctaSecondary}
        <ExternalArrow />
        <span className="sr-only"> (opens in new tab)</span>
      </a>
    </div>
  );
}
