import type { ComponentType } from "react";
import { features } from "@/lib/content";
import { Container } from "./Container";
import { AnalyzeArt } from "./features/AnalyzeArt";
import { CleanArt } from "./features/CleanArt";
import { DoctorArt } from "./features/DoctorArt";
import { FeatureCard } from "./features/FeatureCard";
import { PurgeArt } from "./features/PurgeArt";
import { StatusArt } from "./features/StatusArt";
import { Section, SectionHeader } from "./ui/Section";

/** Bento layout on a 6-column grid: clean leads wide, the rest share the row below. */
const CARDS: Record<string, { Art: ComponentType; span: string }> = {
  clean: { Art: CleanArt, span: "sm:col-span-2 lg:col-span-4" },
  analyze: { Art: AnalyzeArt, span: "lg:col-span-2" },
  purge: { Art: PurgeArt, span: "lg:col-span-2" },
  doctor: { Art: DoctorArt, span: "lg:col-span-2" },
  status: { Art: StatusArt, span: "lg:col-span-2" },
};

export function FeaturesGrid() {
  return (
    <Section id="features" tone="base" labelledBy="features-title">
      <SectionHeader
        id="features-title"
        eyebrow={features.eyebrow}
        title={features.title}
        subhead={features.subhead}
      />
      <Container className="mt-14 sm:mt-16">
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-6">
          {features.cards.map((card) => {
            const entry = CARDS[card.title];
            if (!entry) return null;
            const { Art, span } = entry;
            return (
              <FeatureCard key={card.title} command={card.title} description={card.description} className={span}>
                <Art />
              </FeatureCard>
            );
          })}
        </div>
      </Container>
    </Section>
  );
}
