import { WithFlags } from "../ui/Flags";
import { CheckGlyph } from "../ui/icons";

/** Heading, description and checked points under each visual. */
export function ColumnText({
  label,
  description,
  points,
}: {
  label: string;
  description: string;
  points: string[];
}) {
  return (
    <div>
      <h3 className="text-lg font-semibold tracking-tight text-ink">{label}</h3>
      <p className="mt-2 max-w-md text-pretty leading-relaxed text-ink-muted">
        <WithFlags>{description}</WithFlags>
      </p>
      <ul className="mt-5 grid gap-2.5">
        {points.map((point) => (
          <li key={point} className="flex items-start gap-2.5 text-sm leading-relaxed text-ink-muted">
            <CheckGlyph className="mt-[3px] text-accent-text" />
            <span>
              <WithFlags>{point}</WithFlags>
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
