import type { ReactNode } from "react";
import { Container } from "../Container";
import { WithFlags } from "./Flags";

const TONES = {
  base: "bg-bg",
  soft: "bg-bg-soft",
  deep: "bg-surface-dark",
} as const;

/** Page section: consistent vertical rhythm, tone, and anchor offset. */
export function Section({
  id,
  tone = "base",
  className = "",
  children,
  labelledBy,
}: {
  id?: string;
  tone?: keyof typeof TONES;
  className?: string;
  children: ReactNode;
  labelledBy?: string;
}) {
  return (
    <section
      id={id}
      aria-labelledby={labelledBy}
      className={`relative py-24 sm:py-28 lg:py-36 ${TONES[tone]} ${className}`}
    >
      {children}
    </section>
  );
}

/** Heading block used by every section, so type scale and spacing stay identical. */
export function SectionHeader({
  id,
  eyebrow,
  title,
  subhead,
  align = "left",
  action,
  className = "",
}: {
  id: string;
  eyebrow?: string;
  title: ReactNode;
  subhead?: ReactNode;
  align?: "left" | "center";
  action?: ReactNode;
  className?: string;
}) {
  const centered = align === "center";
  return (
    <Container>
      <div
        className={`flex flex-col gap-6 ${centered ? "items-center text-center" : "md:flex-row md:items-end md:justify-between"} ${className}`}
      >
        <div className={centered ? "mx-auto max-w-2xl" : "max-w-2xl"}>
          {eyebrow && <p className="eyebrow">{eyebrow}</p>}
          <h2
            id={id}
            className="mt-4 text-balance text-[clamp(2rem,4.2vw,3.25rem)] font-semibold leading-[1.05] tracking-[-0.035em] text-ink"
          >
            {title}
          </h2>
          {subhead && (
            <p className="mt-5 max-w-xl text-pretty text-base leading-relaxed text-ink-muted sm:text-lg">
              {typeof subhead === "string" ? <WithFlags>{subhead}</WithFlags> : subhead}
            </p>
          )}
        </div>
        {action && <div className="shrink-0">{action}</div>}
      </div>
    </Container>
  );
}
