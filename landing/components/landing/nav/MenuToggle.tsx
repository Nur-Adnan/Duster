import type { Ref } from "react";
import { Glyph } from "../ui/icons";

const LINE = "[transform-box:fill-box] origin-center transition-[translate,rotate,scale,opacity] duration-200";

/** 44px hamburger that morphs into an X; the lines rotate about their own centers. */
export function MenuToggle({
  open,
  controls,
  onToggle,
  ref,
}: {
  open: boolean;
  controls: string;
  onToggle: () => void;
  ref?: Ref<HTMLButtonElement>;
}) {
  return (
    <button
      ref={ref}
      type="button"
      aria-label="Menu"
      aria-expanded={open}
      aria-controls={controls}
      onClick={onToggle}
      className="-mr-2 flex h-11 w-11 items-center justify-center rounded-md text-ink transition-colors hover:bg-bg-soft lg:hidden"
    >
      <Glyph>
        <line x1="3" y1="5" x2="17" y2="5" className={`${LINE} ${open ? "translate-y-[5px] rotate-45" : ""}`} />
        <line x1="3" y1="10" x2="17" y2="10" className={`${LINE} ${open ? "scale-x-20 opacity-0" : ""}`} />
        <line x1="3" y1="15" x2="17" y2="15" className={`${LINE} ${open ? "-translate-y-[5px] -rotate-45" : ""}`} />
      </Glyph>
    </button>
  );
}
