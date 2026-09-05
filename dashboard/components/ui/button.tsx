import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

/**
 * Button — DESIGN.md § Components.
 *
 * Variants own COLOR + WEIGHT; sizes own GEOMETRY + FONT-SIZE. Keeping those
 * axes disjoint means no `cn()` merge-order surprises.
 *
 * Every variant carries a 1px border (transparent by default) so filled and
 * outlined buttons occupy identical space and never jitter when a variant
 * changes at runtime.
 *
 * `primary` (acid lime) is the one chromatic control in the system and is
 * deliberately NOT the default — DESIGN.md § Do's: "Use #e4f222 exclusively
 * for the single primary action per view". Opt-in makes that rule structural
 * rather than something to police in review.
 *
 * `destructive` is a coral OUTLINE, not a fill. The spec bars extra chromatic
 * action colors and calls Coral Red "a supporting accent, not a status color",
 * so the filled treatment survives only as `destructive-solid` for the final
 * confirm inside a dialog (see ConfirmDialog).
 */
const buttonVariants = cva(
  cn(
    "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md border border-transparent transition-colors",
    "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
    "disabled:pointer-events-none disabled:opacity-50",
    "[&_svg]:size-3.5 [&_svg]:shrink-0"
  ),
  {
    variants: {
      variant: {
        /**
         * THE accent. Carries the spec's inset shadow stack — DESIGN.md
         * § Elevation calls this "the only place in the system where a real
         * shadow is applied to a chrome element", so it survives in both
         * themes. Acid lime scores 1.2:1 on white, so light mode adds a dark
         * hairline for edge definition; dark mode drops it because lime
         * already reads as a flare against the void.
         */
        primary:
          "border-void/15 bg-primary font-w510 text-primary-foreground shadow-subtle-3 hover:bg-primary/90 dark:border-transparent",
        /** § Ghost / Outline Button — the workhorse, hence the default. */
        outline:
          "border-border bg-transparent font-normal text-foreground hover:bg-accent hover:text-accent-foreground",
        /** Filled but achromatic — the graphite interactive tint. */
        secondary: "bg-secondary font-normal text-secondary-foreground hover:bg-secondary/80",
        ghost:
          "bg-transparent font-normal text-muted-foreground hover:bg-accent hover:text-accent-foreground",
        /** Coral outline. Reads as dangerous without claiming the accent slot. */
        destructive:
          "border-destructive/40 bg-transparent font-normal text-destructive hover:border-destructive/60 hover:bg-destructive/10",
        /** Filled coral — confirm step only, never a page-level trigger. */
        "destructive-solid":
          "bg-destructive font-w510 text-destructive-foreground hover:bg-destructive/90",
        link: "bg-transparent font-normal text-foreground underline-offset-4 hover:underline",
      },
      size: {
        /** § Ghost / Outline Button: padding 8px 12px, 13px. */
        default: "h-8 px-3 py-1.5 text-caption",
        sm: "h-7 px-2.5 text-label",
        /** § Primary Action Button: padding 10px 16px, 14px. */
        lg: "h-10 px-4 text-sm",
        icon: "h-8 w-8 p-0",
      },
    },
    defaultVariants: { variant: "outline", size: "default" },
  }
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
  loading?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, loading, children, disabled, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    if (asChild) {
      // Slot requires a single element child; spinner can't be a sibling.
      return (
        <Comp ref={ref} className={cn(buttonVariants({ variant, size }), className)} {...props}>
          {children}
        </Comp>
      );
    }
    return (
      <Comp
        ref={ref}
        className={cn(buttonVariants({ variant, size }), className)}
        disabled={disabled || loading}
        aria-busy={loading || undefined}
        {...props}
      >
        {loading && (
          <svg className="h-3.5 w-3.5 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden>
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
          </svg>
        )}
        {children}
      </Comp>
    );
  }
);
Button.displayName = "Button";

export { Button, buttonVariants };
