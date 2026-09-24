import { hero } from "@/lib/content";
import { DiskSweep } from "./hero/DiskSweep";
import { HeroCtas } from "./hero/HeroCtas";
import { HeroStage } from "./hero/HeroStage";
import { InstallCommand } from "./hero/InstallCommand";
import { Rise, RiseLine } from "./hero/Rise";

export function Hero() {
  const copy = (
    <div className="max-w-xl">
      <Rise delay={0}>
        <p className="eyebrow">{hero.eyebrow}</p>
      </Rise>

      <h1
        id="hero-title"
        className="mt-6 text-[clamp(2.75rem,8.5vw,5.5rem)] leading-[0.98] font-semibold tracking-[-0.045em] text-balance text-ink lg:text-[clamp(3.25rem,5.6vw,5.5rem)]"
      >
        {hero.headline.map((line, i) => (
          <RiseLine key={line} delay={0.08 + i * 0.1}>
            {line}
          </RiseLine>
        ))}
      </h1>

      <Rise delay={0.3}>
        <p className="mt-6 max-w-136 text-pretty text-base leading-relaxed text-ink-muted sm:text-lg">
          {hero.subhead}
        </p>
      </Rise>

      <Rise delay={0.4} className="mt-9">
        <HeroCtas />
      </Rise>

      <Rise delay={0.5} className="mt-8">
        <InstallCommand />
      </Rise>
    </div>
  );

  const visual = (
    <Rise delay={0.35}>
      <DiskSweep />
    </Rise>
  );

  return <HeroStage copy={copy} visual={visual} />;
}
