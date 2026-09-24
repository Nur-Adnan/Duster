import { faq, links } from "@/lib/content";
import { Container } from "./Container";
import { FaqAccordion } from "./faq/FaqAccordion";
import { IssueCard } from "./faq/IssueCard";
import { ArrowIcon } from "./ui/icons";
import { Section } from "./ui/Section";

export function FAQ() {
  return (
    <Section id="faq" tone="base" labelledBy="faq-title">
      <Container className="grid gap-12 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-20">
        <div className="lg:sticky lg:top-[calc(var(--nav-h)+2.5rem)] lg:self-start">
          <p className="eyebrow">FAQ</p>
          <h2
            id="faq-title"
            className="mt-4 max-w-md text-balance text-[clamp(2rem,4.2vw,3.25rem)] font-semibold leading-[1.05] tracking-[-0.035em] text-ink"
          >
            {faq.title}
          </h2>

          {/* Stacked on phones and at lg (narrow sticky column); side by side at md. */}
          <div className="mt-10 max-w-md rounded-xl border border-hairline bg-bg-soft p-5 sm:p-6 md:grid md:max-w-none md:grid-cols-2 md:items-center md:gap-8 lg:block lg:max-w-md">
            <IssueCard />
            <div className="mt-5 md:mt-0 lg:mt-5">
              <p className="text-base font-semibold text-ink">{faq.calloutText}</p>
              <a
                href={links.newIssue}
                target="_blank"
                rel="noreferrer"
                className="group -my-2 inline-flex min-h-11 items-center gap-2 text-sm font-medium text-accent-text transition-colors hover:text-ink"
              >
                {faq.calloutLink}
                <span className="sr-only"> (opens in new tab)</span>
                <ArrowIcon />
              </a>
            </div>
          </div>
        </div>

        <FaqAccordion items={faq.items} />
      </Container>
    </Section>
  );
}
