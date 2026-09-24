"use client";

import { buttonVariants } from "@heroui/react/button";
import { AnimatePresence, motion } from "framer-motion";
import {
  type FocusEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { links, nav } from "@/lib/content";
import { duration, ease, spring } from "@/lib/motion";
import { Container } from "./Container";
import { Logo } from "./Logo";
import { MenuToggle } from "./nav/MenuToggle";
import { useActiveSection } from "./nav/useActiveSection";

const DRAWER_ID = "mobile-menu";
const SECTION_IDS = nav.links.map((link) => link.href.slice(1));

export function Nav() {
  const [scrolled, setScrolled] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const active = useActiveSection(SECTION_IDS);

  const toggleRef = useRef<HTMLButtonElement>(null);
  const drawerRef = useRef<HTMLDivElement>(null);
  const firstLinkRef = useRef<HTMLAnchorElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // A 1px sentinel 40px down the document: the nav is "scrolled" once it leaves
  // the viewport. The observer fires only on crossings (and once on mount, so a
  // reload mid-page starts in the right state), never per scroll frame.
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el) return;
    const observer = new IntersectionObserver(([entry]) =>
      setScrolled(!entry.isIntersecting),
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const closeDrawer = useCallback((returnFocus: boolean) => {
    setDrawerOpen(false);
    if (returnFocus) toggleRef.current?.focus({ preventScroll: true });
  }, []);

  useEffect(() => {
    if (!drawerOpen) return;
    firstLinkRef.current?.focus({ preventScroll: true });
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") closeDrawer(true);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [drawerOpen, closeDrawer]);

  // Tabbing out of the drawer (anywhere but back to the toggle) closes it.
  const onDrawerBlur = (e: FocusEvent<HTMLDivElement>) => {
    const next = e.relatedTarget as Node | null;
    if (!next) return;
    if (drawerRef.current?.contains(next) || toggleRef.current?.contains(next))
      return;
    closeDrawer(false);
  };

  return (
    <>
      <div
        ref={sentinelRef}
        aria-hidden="true"
        className="pointer-events-none absolute left-0 top-10 h-px w-px"
      />
      <header
        className={`fixed inset-x-0 top-0 z-50 border-b transition-colors duration-300 ${
          scrolled || drawerOpen
            ? "border-hairline bg-bg/80 backdrop-blur-md"
            : "border-transparent bg-transparent"
        }`}
      >
        <Container className="flex h-(--nav-h) items-center justify-between gap-6">
          <a href="#top" className="-ml-1 rounded-md px-1 py-1.5">
            <Logo />
          </a>

          <nav aria-label="Primary" className="hidden lg:block">
            <ul className="flex items-center gap-1">
              {nav.links.map((link) => {
                const isActive = active === link.href.slice(1);
                return (
                  <li key={link.href}>
                    <a
                      href={link.href}
                      aria-current={isActive ? "true" : undefined}
                      className={`relative inline-flex h-11 items-center rounded-md px-3 text-sm font-medium transition-colors ${
                        isActive ? "text-ink" : "text-ink-muted hover:text-ink"
                      }`}
                    >
                      {link.label}
                      {isActive && (
                        <motion.span
                          layoutId="nav-active-indicator"
                          aria-hidden="true"
                          transition={spring}
                          className="absolute inset-x-3 bottom-1.5 h-0.5 rounded-full bg-brand-accent"
                        />
                      )}
                    </a>
                  </li>
                );
              })}
            </ul>
          </nav>

          <div className="hidden items-center lg:flex">
            <a
              href={links.latestRelease}
              target="_blank"
              rel="noreferrer"
              className={`${buttonVariants({ variant: "primary", size: "md" })} min-h-11 rounded-md px-5`}
            >
              {nav.cta}
              <span className="sr-only"> (opens in new tab)</span>
            </a>
          </div>

          <MenuToggle
            ref={toggleRef}
            open={drawerOpen}
            controls={DRAWER_ID}
            onToggle={() =>
              drawerOpen ? closeDrawer(true) : setDrawerOpen(true)
            }
          />
        </Container>

        <AnimatePresence>
          {drawerOpen && (
            <motion.div
              ref={drawerRef}
              id={DRAWER_ID}
              onBlur={onDrawerBlur}
              initial={{ opacity: 0, y: -8 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -8 }}
              transition={{ duration: duration.fast, ease: ease.out }}
              className="absolute inset-x-0 top-full border-b border-hairline bg-bg lg:hidden"
            >
              <Container className="py-3">
                <nav aria-label="Mobile">
                  <ul className="flex flex-col">
                    {nav.links.map((link, i) => {
                      const isActive = active === link.href.slice(1);
                      return (
                        <li key={link.href}>
                          <a
                            ref={i === 0 ? firstLinkRef : undefined}
                            href={link.href}
                            aria-current={isActive ? "true" : undefined}
                            onClick={() => closeDrawer(true)}
                            className={`flex h-11 items-center gap-3 rounded-md px-3 text-sm font-medium transition-colors hover:bg-bg-soft ${
                              isActive
                                ? "text-ink"
                                : "text-ink-muted hover:text-ink"
                            }`}
                          >
                            <span
                              aria-hidden="true"
                              className={`h-4 w-0.5 rounded-full transition-colors ${
                                isActive ? "bg-brand-accent" : "bg-transparent"
                              }`}
                            />
                            {link.label}
                          </a>
                        </li>
                      );
                    })}
                  </ul>
                </nav>
                <div className="mt-3 border-t border-hairline pt-3">
                  <a
                    href={links.latestRelease}
                    target="_blank"
                    rel="noreferrer"
                    onClick={() => closeDrawer(false)}
                    className={`${buttonVariants({ variant: "primary", size: "md", fullWidth: true })} min-h-11 rounded-md`}
                  >
                    {nav.cta}
                    <span className="sr-only"> (opens in new tab)</span>
                  </a>
                </div>
              </Container>
            </motion.div>
          )}
        </AnimatePresence>
      </header>
    </>
  );
}
