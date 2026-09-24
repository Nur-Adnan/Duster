import { spotlight } from "@/lib/content";
import { Container } from "./Container";
import { PipelineStack } from "./spotlight/PipelineStack";
import { SafetyPipeline } from "./spotlight/SafetyPipeline";
import { WithFlags } from "./ui/Flags";
import { Glyph } from "./ui/icons";
import { Section, SectionHeader } from "./ui/Section";

/** Preview: an eye inside a dashed frame, nothing touched yet. */
function PreviewGlyph() {
  return (
    <Glyph viewBox="0 0 24 24">
      <rect x="2.75" y="4.75" width="18.5" height="14.5" rx="2.5" stroke="var(--ink-faint)" strokeDasharray="2 2" />
      <path d="M6.5 12s2.2-3.5 5.5-3.5 5.5 3.5 5.5 3.5-2.2 3.5-5.5 3.5-5.5-3.5-5.5-3.5z" stroke="var(--accent-text)" />
      <circle cx="12" cy="12" r="1.5" stroke="var(--accent-text)" />
    </Glyph>
  );
}

/** Logged: a log file whose newest line is the one being written. */
function LogGlyph() {
  return (
    <Glyph viewBox="0 0 24 24">
      <path d="M5.5 2.75h8.5l4.5 4.5v7.5M11 21.25H5.5V2.75" stroke="var(--ink-faint)" />
      <path d="M14 2.75v4.5h4.5M8.5 11h7M8.5 14.5h3" stroke="var(--ink-faint)" />
      <path d="m14.5 19 2 2 4-4.25" stroke="var(--ok)" />
    </Glyph>
  );
}

const GLYPHS = [PreviewGlyph, LogGlyph];

export function FeatureSpotlight() {
  return (
    <Section id="spotlight" tone="deep" labelledBy="spotlight-title">
      <SectionHeader
        id="spotlight-title"
        eyebrow={spotlight.eyebrow}
        title={spotlight.title}
        subhead={spotlight.body}
      />

      <Container className="mt-14 lg:mt-16">
        <div className="hidden lg:block">
          <SafetyPipeline />
        </div>
        <div className="lg:hidden">
          <PipelineStack />
        </div>

        <div className="mx-auto mt-14 grid max-w-2xl gap-10 border-t border-hairline pt-12 lg:mt-16 lg:max-w-none lg:grid-cols-2 lg:gap-16">
          {spotlight.supporting.map((item, i) => {
            const Icon = GLYPHS[i % GLYPHS.length];
            return (
              <div key={item.title} className="flex gap-5">
                <span className="grid h-11 w-11 shrink-0 place-items-center rounded-lg border border-hairline bg-bg-raised">
                  <Icon />
                </span>
                <div>
                  <h3 className="pt-2 text-lg font-semibold tracking-tight text-ink">{item.title}</h3>
                  <p className="mt-2 max-w-md text-[15px] leading-relaxed text-ink-muted">
                    <WithFlags>{item.body}</WithFlags>
                  </p>
                </div>
              </div>
            );
          })}
        </div>
      </Container>
    </Section>
  );
}
