import { DeveloperSection } from "@/components/landing/DeveloperSection";
import { EnterpriseSection } from "@/components/landing/EnterpriseSection";
import { FAQ } from "@/components/landing/FAQ";
import { FeatureSpotlight } from "@/components/landing/FeatureSpotlight";
import { FeaturesGrid } from "@/components/landing/FeaturesGrid";
import { FlexibleUsage } from "@/components/landing/FlexibleUsage";
import { Footer } from "@/components/landing/Footer";
import { Hero } from "@/components/landing/Hero";
import { IntegrationsSection } from "@/components/landing/IntegrationsSection";
import { Nav } from "@/components/landing/Nav";
import { Showcase } from "@/components/landing/Showcase";

export default function Home() {
  return (
    <>
      <Nav />
      <main id="main">
        <Hero />
        <FeaturesGrid />
        <FeatureSpotlight />
        <Showcase />
        <FlexibleUsage />
        <DeveloperSection />
        <EnterpriseSection />
        <IntegrationsSection />
        <FAQ />
      </main>
      <Footer />
    </>
  );
}
