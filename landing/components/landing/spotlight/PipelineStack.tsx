"use client";

import { motion } from "framer-motion";
import { fadeUp, stagger } from "@/lib/motion";
import { WithFlags } from "../ui/Flags";
import { FATE_STYLES, GATES, PIPELINE_CAPTION, TOKENS, type Token } from "./data";
import { FateIcon, GateGlyph } from "./glyphs";

function TokenCard({ token }: { token: Token }) {
  const style = FATE_STYLES[token.fate];
  return (
    <div className={`mt-3 rounded-lg border p-3 ${style.card}`}>
      <code className="block break-all font-mono text-[13px] leading-relaxed text-ink">{token.path}</code>
      <p className={`mt-2 flex items-center gap-2 text-sm font-medium ${style.text}`}>
        <FateIcon fate={token.fate} className="h-4 w-4 shrink-0" />
        {token.fateLabel}
      </p>
    </div>
  );
}

/**
 * Below lg: the pipeline as a static vertical track, already in its final
 * state. Each example path sits at the gate that stopped it.
 */
export function PipelineStack() {
  const survivor = TOKENS.filter((t) => t.stopGate === null);

  return (
    <div className="mx-auto max-w-2xl">
      <p className="text-sm leading-relaxed text-ink-muted">{PIPELINE_CAPTION}</p>

      <motion.div
        role="list"
        aria-label="Safety gates, in order"
        initial="hidden"
        whileInView="visible"
        viewport={{ once: true, margin: "-80px" }}
        variants={stagger()}
        className="relative mt-8"
      >
        {GATES.map((gate, g) => (
          <motion.div key={gate.id} role="listitem" variants={fadeUp} className="relative pb-8 pl-16">
            <span aria-hidden="true" className="absolute bottom-1 left-[21px] top-[52px] w-0.5 rounded-full bg-hairline-strong" />
            <span
              aria-hidden="true"
              className="absolute left-0 top-0 grid h-11 w-11 place-items-center rounded-lg border border-hairline bg-bg-raised text-accent-text"
            >
              <GateGlyph id={gate.id} className="h-5 w-5" />
            </span>
            <p className="pt-2.5 text-[15px] font-medium leading-6 text-ink">{gate.label}</p>
            <p className="mt-1 text-sm leading-relaxed text-ink-muted">
              <WithFlags>{gate.note}</WithFlags>
            </p>
            {TOKENS.filter((t) => t.stopGate === g).map((t) => (
              <TokenCard key={t.path} token={t} />
            ))}
          </motion.div>
        ))}

        <motion.div role="listitem" variants={fadeUp} className="relative pl-16">
          <span
            aria-hidden="true"
            className={`absolute left-0 top-0 grid h-11 w-11 place-items-center rounded-full border ${FATE_STYLES.ok.ring}`}
          >
            <FateIcon fate="ok" className="h-5 w-5" />
          </span>
          <p className="pt-2.5 text-[15px] font-medium leading-6 text-ink">Through every gate</p>
          {survivor.map((t) => (
            <TokenCard key={t.path} token={t} />
          ))}
        </motion.div>
      </motion.div>
    </div>
  );
}
