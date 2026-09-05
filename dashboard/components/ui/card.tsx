import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * Card — DESIGN.md § Card (Product Screenshot Frame): "Background #0f1011,
 * border-radius 12px, inset shadow rgb(35,37,42) 0 0 0 1px, padding 24px.
 * Hairline inner border defines the card edge — no outer shadow, no glow."
 *
 * Implemented as a real 1px border rather than the spec's inset box-shadow,
 * for two reasons:
 *   1. § Do's sanctions exactly this — "Use hairline borders (#23252a or
 *      #383b3f) instead of shadows for surface separation".
 *   2. The `--shadow-subtle` token hardcodes graphite `rgb(35,37,42)`, which is
 *      correct on the dark canvas but wrong on a white one. `border-border` is
 *      theme-aware and resolves to #e5e6e8 in light mode.
 * It also keeps `hover:border-*` affordances working at call sites, which an
 * inset shadow would silently break (zero border-width = invisible color).
 */
const Card = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      className={cn("rounded-lg border border-border bg-card text-card-foreground", className)}
      {...props}
    />
  )
);
Card.displayName = "Card";

/** 24px card padding is the spec's § Layout value; 16px below `sm`. */
const CardHeader = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div ref={ref} className={cn("flex flex-col gap-1 p-4 sm:p-6", className)} {...props} />
  )
);
CardHeader.displayName = "CardHeader";

/** Headings are the one place pure white is allowed (§ Quick Color Reference). */
const CardTitle = React.forwardRef<HTMLParagraphElement, React.HTMLAttributes<HTMLHeadingElement>>(
  ({ className, ...props }, ref) => (
    <h3
      ref={ref}
      className={cn("text-sm font-w510 leading-none tracking-snug text-foreground-strong", className)}
      {...props}
    />
  )
);
CardTitle.displayName = "CardTitle";

const CardDescription = React.forwardRef<HTMLParagraphElement, React.HTMLAttributes<HTMLParagraphElement>>(
  ({ className, ...props }, ref) => (
    <p ref={ref} className={cn("text-caption text-muted-foreground", className)} {...props} />
  )
);
CardDescription.displayName = "CardDescription";

const CardContent = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div ref={ref} className={cn("p-4 pt-0 sm:p-6 sm:pt-0", className)} {...props} />
  )
);
CardContent.displayName = "CardContent";

const CardFooter = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div ref={ref} className={cn("flex items-center p-4 pt-0 sm:p-6 sm:pt-0", className)} {...props} />
  )
);
CardFooter.displayName = "CardFooter";

export { Card, CardHeader, CardFooter, CardTitle, CardDescription, CardContent };
