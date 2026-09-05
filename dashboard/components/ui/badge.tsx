import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

/**
 * Badge — DESIGN.md § Badge / Status Tag: "Background rgba(255,255,255,0.05),
 * text #8a8f98, border-radius 4px, padding 0px 6px, Inter 12px / weight 400."
 *
 * Two shapes, two jobs:
 *   - `Badge`       → 4px square-ish tag. Inline metadata, categories, engine
 *                     names. Filled, borderless.
 *   - `StatusBadge` → 9999px pill with a liveness dot (§ Pill Button). Reserved
 *                     for live state: connected / enabled / degraded.
 *
 * Color-coded variants draw only from the spec's supporting accents (Pulse
 * Green, Coral Red, Iris Violet, Lavender). Acid lime is NOT available here —
 * it belongs to the single primary action per view.
 *
 * `warning` is intentionally achromatic. The spec has no amber, and promoting
 * a warning to the accent would break the one-chromatic-element rule. Callers
 * asking for `warning` get the neutral treatment.
 */
const badgeVariants = cva(
  "inline-flex max-w-full items-center gap-1.5 whitespace-nowrap rounded-badge border border-transparent px-1.5 py-0.5 text-label font-normal transition-colors",
  {
    variants: {
      variant: {
        /** Neutral metadata — the spec's baseline tag. */
        default: "bg-foreground/[0.06] text-foreground",
        secondary: "bg-foreground/[0.06] text-muted-foreground",
        muted: "bg-foreground/[0.06] text-muted-foreground",
        /** Achromatic by design — see the note above. */
        warning: "bg-foreground/[0.06] text-muted-foreground",
        outline: "border-border bg-transparent text-muted-foreground",
        /** Supporting accents, tinted fill + matching text. */
        success: "bg-success/15 text-success",
        destructive: "bg-destructive/15 text-destructive",
        info: "bg-info/15 text-info",
        tag: "bg-iris-violet/15 text-iris-violet",
        category: "bg-lavender/15 text-lavender",
      },
    },
    defaultVariants: { variant: "default" },
  }
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, children, ...props }: BadgeProps) {
  return (
    <span className={cn(badgeVariants({ variant }), className)} {...props}>
      <span className="truncate">{children}</span>
    </span>
  );
}

/**
 * Dot tones. `info` uses Signal Teal (the spec's "informational icon fill")
 * rather than plain foreground, so an info dot is distinguishable from a
 * neutral one.
 */
const dotTones = {
  success: "bg-success",
  warning: "bg-muted-foreground",
  destructive: "bg-destructive",
  muted: "bg-muted-foreground",
  info: "bg-info",
} as const;

/**
 * Live-state pill: outlined, 9999px, with a colored liveness dot —
 * e.g. `connected`, `enabled`, `paused`. DESIGN.md § Pill Button.
 * For static metadata use `Badge` instead; the pill shape signals "this
 * value can change while you watch it".
 */
function StatusBadge({
  tone = "muted",
  children,
  className,
}: {
  tone?: keyof typeof dotTones;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex max-w-full items-center gap-1.5 whitespace-nowrap rounded-full border border-border bg-transparent px-2 py-0.5 text-label font-normal text-muted-foreground",
        className
      )}
    >
      <span aria-hidden className={cn("h-1.5 w-1.5 shrink-0 rounded-full", dotTones[tone])} />
      <span className="truncate">{children}</span>
    </span>
  );
}

export { Badge, StatusBadge, badgeVariants };
