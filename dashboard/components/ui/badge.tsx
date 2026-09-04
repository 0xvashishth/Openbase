import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex max-w-full items-center gap-1.5 whitespace-nowrap rounded-full border bg-transparent px-2 py-0.5 text-xs font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2",
  {
    variants: {
      variant: {
        default: "border-foreground/25 text-foreground",
        secondary: "border-border text-muted-foreground",
        outline: "border-input text-foreground",
        destructive: "border-destructive/50 text-destructive",
        success: "border-success/50 text-success",
        warning: "border-warning/50 text-warning",
        muted: "border-border text-muted-foreground",
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

const dotTones = {
  success: "bg-success",
  warning: "bg-warning",
  destructive: "bg-destructive",
  muted: "bg-muted-foreground",
  info: "bg-foreground",
} as const;

/**
 * Supabase-style status pill: outlined with a colored liveness dot,
 * e.g. `connected`, `paused`, `error`. Monochrome-friendly.
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
        "inline-flex max-w-full items-center gap-1.5 whitespace-nowrap rounded-full border border-border bg-transparent px-2 py-0.5 text-xs font-medium text-muted-foreground",
        className
      )}
    >
      <span aria-hidden className={cn("h-1.5 w-1.5 shrink-0 rounded-full", dotTones[tone])} />
      <span className="truncate">{children}</span>
    </span>
  );
}

export { Badge, StatusBadge, badgeVariants };
