import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center rounded-md border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2",
  {
    variants: {
      variant: {
        default: "border-transparent bg-primary text-primary-foreground",
        secondary: "border-transparent bg-secondary text-secondary-foreground",
        outline: "text-foreground",
        destructive: "border-transparent bg-destructive text-destructive-foreground",
        success: "border-transparent bg-success text-success-foreground",
        warning: "border-transparent bg-warning text-warning-foreground",
        muted: "border-transparent bg-muted text-muted-foreground",
      },
    },
    defaultVariants: { variant: "default" },
  }
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}

/** Legacy tone prop used by older code: maps to the monochrome variants above. */
export function LegacyBadge({
  tone = "slate",
  children,
  className,
}: {
  tone?: "green" | "amber" | "red" | "slate" | "blue";
  children: React.ReactNode;
  className?: string;
}) {
  const map = { green: "success", amber: "warning", red: "destructive", slate: "muted", blue: "secondary" } as const;
  return (
    <Badge variant={map[tone]} className={className}>
      {children}
    </Badge>
  );
}

export { Badge, badgeVariants };
