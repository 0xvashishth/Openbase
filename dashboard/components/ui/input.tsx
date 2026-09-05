import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * Input — DESIGN.md § Text Input: "Background rgba(255,255,255,0.02), border
 * 1px rgba(255,255,255,0.08), text #d0d6e0, border-radius 6px, padding
 * 12px 14px, Inter 14px / weight 400. Focus ring: border brightens to #d0d6e0."
 *
 * Focus BRIGHTENS THE EDGE instead of drawing a ring — that is the spec's
 * stated mechanism, and it also keeps the 6px geometry intact (a ring at this
 * radius reads as a halo). The `focus-visible:ring-0` is defensive: it stops an
 * inherited ring utility from reintroducing the halo.
 *
 * Height is 32px to sit flush with `Button` size="default"; the spec's 12px
 * vertical padding is honoured on the `lg`-height textarea variants at call
 * sites, where multi-line content needs the breathing room.
 */
const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => (
    <input
      type={type}
      ref={ref}
      className={cn(
        "flex h-8 w-full rounded-md border border-input bg-foreground/[0.02] px-2.5 py-1 text-caption text-foreground transition-colors",
        "file:border-0 file:bg-transparent file:text-caption file:font-w510",
        "placeholder:text-muted-foreground/70",
        "focus-visible:border-ring focus-visible:outline-none focus-visible:ring-0",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className
      )}
      {...props}
    />
  )
);
Input.displayName = "Input";

export { Input };
