import { enterprise } from "@/lib/content";
import { Container } from "./Container";
import { EnterpriseCards } from "./enterprise/EnterpriseCards";
import { FleetDiagram } from "./enterprise/FleetDiagram";
import { LifecycleStrip } from "./enterprise/LifecycleStrip";
import { Section, SectionHeader } from "./ui/Section";

export function EnterpriseSection() {
  return (
    <Section id="enterprise" tone="soft" labelledBy="enterprise-title">
      <SectionHeader
        id="enterprise-title"
        eyebrow={enterprise.eyebrow}
        title={enterprise.title}
        subhead={enterprise.subhead}
      />
      <Container className="mt-12 sm:mt-16">
        <div className="grid gap-4 lg:grid-cols-12 lg:gap-6">
          <div className="overflow-hidden rounded-xl border border-hairline bg-bg lg:col-span-7">
            <FleetDiagram />
            <LifecycleStrip />
          </div>
          <EnterpriseCards />
        </div>
      </Container>
    </Section>
  );
}
