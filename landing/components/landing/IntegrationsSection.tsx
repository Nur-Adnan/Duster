import { integrations } from "@/lib/content";
import { Container } from "./Container";
import { Pipeline } from "./pipeline/Pipeline";
import { RunCard } from "./pipeline/RunCard";
import { Section, SectionHeader } from "./ui/Section";

const HEADING_ID = "integrations-title";

// The Sign stage is only live once the signing job is. Read it from the same
// status rows the run card shows, so the two can never disagree.
const signing = integrations.statusRows.find((r) => /sign/i.test(r.label));
const PENDING_STAGES = signing && signing.status !== "done" ? ["Sign"] : [];

export function IntegrationsSection() {
  return (
    <Section id="release-pipeline" tone="deep" labelledBy={HEADING_ID} className="overflow-hidden">
      {/* Faint engineering grid, fading out toward the edges of the band. */}
      <div
        aria-hidden="true"
        className="bg-grid pointer-events-none absolute inset-0 opacity-60 mask-[radial-gradient(ellipse_70%_60%_at_50%_40%,black,transparent)]"
      />

      <div className="relative">
        <SectionHeader
          id={HEADING_ID}
          eyebrow={integrations.eyebrow}
          title={integrations.title}
          subhead={integrations.subhead}
        />

        <Container className="mt-12 sm:mt-14 lg:mt-16">
          <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,22rem)] lg:gap-8">
            <Pipeline stages={integrations.tiles} pending={PENDING_STAGES} />
            <RunCard rows={integrations.statusRows} />
          </div>
        </Container>
      </div>
    </Section>
  );
}
