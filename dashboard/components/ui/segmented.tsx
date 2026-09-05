"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * Segmented single-choice option — a toggle button in an `aria-pressed` group.
 *
 * The selected state marks the EDGE with the accent, the same "active
 * indicator" language used by tabs and sidebar nav, rather than filling with it.
 * A lime fill here would read as the page's primary action and break the
 * one-chromatic-element rule (DESIGN.md § Don't).
 *
 * Extracted because DBSourcePanel, FunctionsPanel and TriggersPanel each
 * hand-rolled the same template-literal variant of this, which had already
 * drifted apart (three paddings, two radii).
 */
export function SegmentedOption({
  selected,
  onClick,
  className,
  children,
}: {
  selected: boolean;
  onClick: () => void;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={selected}
      className={cn(
        "rounded-md border px-3 py-1.5 text-caption transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
        selected
          ? "border-primary bg-primary/10 font-w510 text-foreground-strong"
          : "border-border bg-transparent font-normal text-muted-foreground hover:bg-accent hover:text-accent-foreground",
        className
      )}
    >
      {children}
    </button>
  );
}
