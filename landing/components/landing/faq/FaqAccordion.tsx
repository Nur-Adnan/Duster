"use client";

import { Disclosure, DisclosureGroup } from "@heroui/react";
import { WithFlags } from "../ui/Flags";

type Item = { q: string; a: string };

/**
 * Single-expand accordion. Rows are hairline-separated; the only motion is
 * HeroUI's own panel expand/collapse and the chevron's rotation.
 */
export function FaqAccordion({ items }: { items: Item[] }) {
  return (
    <DisclosureGroup
      defaultExpandedKeys={["faq-0"]}
      className="border-t border-hairline"
    >
      {items.map((item, i) => (
        <Disclosure key={item.q} id={`faq-${i}`} className="group/row border-b border-hairline">
          <Disclosure.Heading className="m-0">
            <Disclosure.Trigger className="flex min-h-14 w-full items-center justify-between gap-6 py-5 text-left text-base font-medium leading-snug text-ink data-[focus-visible=true]:ring-0 sm:text-[1.0625rem]">
              <span className="text-pretty">{item.q}</span>
              <span
                aria-hidden="true"
                className="grid size-8 shrink-0 place-items-center rounded-full border border-hairline text-ink-muted transition-colors duration-200 group-hover/row:border-hairline-strong group-hover/row:text-ink group-data-[expanded=true]/row:border-accent-text/40 group-data-[expanded=true]/row:bg-brand-accent/15 group-data-[expanded=true]/row:text-accent-text"
              >
                <Disclosure.Indicator>
                  <svg
                    viewBox="0 0 16 16"
                    fill="none"
                    className="ms-0 size-3.5 duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]"
                  >
                    <path
                      d="M4 6l4 4 4-4"
                      stroke="currentColor"
                      strokeWidth="1.5"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    />
                  </svg>
                </Disclosure.Indicator>
              </span>
            </Disclosure.Trigger>
          </Disclosure.Heading>
          <Disclosure.Content className="duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]">
            <p className="max-w-[62ch] pb-6 pr-10 text-[0.9375rem] leading-relaxed text-ink-muted sm:pr-14">
              <WithFlags chip>{item.a}</WithFlags>
            </p>
          </Disclosure.Content>
        </Disclosure>
      ))}
    </DisclosureGroup>
  );
}
