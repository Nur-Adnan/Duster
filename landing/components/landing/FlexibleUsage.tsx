import { flexibleUsage } from "@/lib/content";
import { Container } from "./Container";
import { Section, SectionHeader } from "./ui/Section";
import { ColumnText } from "./usage/check";
import { CleanTuiColumn } from "./usage/CleanTuiColumn";
import { PurgeJsonTerminal } from "./usage/PurgeJsonTerminal";

// Each column spans both rows of the parent grid via subgrid, so the two
// windows share one height and the headings under them line up on md+.
const COLUMN = "flex min-w-0 flex-col gap-8 md:row-span-2 md:grid md:grid-rows-subgrid";

export function FlexibleUsage() {
  const [interactive, scripted] = flexibleUsage.columns;

  return (
    <Section id="usage" tone="soft" labelledBy="usage-title">
      <SectionHeader
        id="usage-title"
        eyebrow={flexibleUsage.eyebrow}
        title={flexibleUsage.title}
        subhead={flexibleUsage.subhead}
      />

      <Container className="mt-14 sm:mt-16">
        <div className="grid gap-16 md:grid-cols-2 md:gap-x-8 md:gap-y-8 lg:gap-x-12">
          <CleanTuiColumn className={COLUMN}>
            <ColumnText {...interactive} />
          </CleanTuiColumn>

          <article className={COLUMN}>
            <PurgeJsonTerminal />
            <ColumnText {...scripted} />
          </article>
        </div>
      </Container>
    </Section>
  );
}
