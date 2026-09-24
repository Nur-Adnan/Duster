import Image from "next/image";

/** Mark plus wordmark. The image is decorative; the visible "Duster" names it. */
export function Logo() {
  return (
    <span className="flex items-center gap-2.5">
      <Image src="/duster-icon.png" alt="" aria-hidden="true" width={32} height={32} className="h-8 w-8 rounded-full" priority />
      <span className="text-base font-semibold tracking-tight text-ink">Duster</span>
    </span>
  );
}
