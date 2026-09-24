import { links, showcase } from "@/lib/content";
import { Container } from "./Container";
import { ArrowIcon } from "./ui/icons";
import { DiskBar } from "./features/DiskBar";
import { Section, SectionHeader } from "./ui/Section";

export function Showcase() {
  return (
    <Section id="showcase" tone="base" labelledBy="showcase-title">
      <SectionHeader
        id="showcase-title"
        eyebrow={showcase.eyebrow}
        title={showcase.title}
        subhead={showcase.subhead}
        action={
          <a
            href={links.categories}
            target="_blank"
            rel="noreferrer"
            className="group inline-flex min-h-11 items-center gap-1.5 text-sm font-medium text-accent-text transition-colors hover:text-ink"
          >
            {showcase.linkText}
            <span className="sr-only"> (opens in new tab)</span>
            <ArrowIcon />
          </a>
        }
      />
      <Container className="mt-12 sm:mt-14">
        <DiskBar tiles={showcase.tiles} />
      </Container>
    </Section>
  );
}
