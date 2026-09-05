import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * Standard page container.
 *
 * DESIGN.md § Layout pins the content column at 1200px (`max-w-shell`). Before
 * this existed, page containers were copy-pasted 20+ times in three widths
 * (max-w-5xl / 6xl / 4xl), and `ProjectGuard` re-declared its own — so a
 * blocked or loading tool page rendered container-inside-container and got
 * 48px of padding instead of 24px.
 *
 * Rule: exactly one PageShell per rendered view. Components that render *inside*
 * a page (guards, panels, empty states) must not add their own.
 */
export function PageShell({
  className,
  children,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("mx-auto w-full max-w-shell px-4 py-6 sm:px-6", className)}
      {...props}
    >
      {children}
    </div>
  );
}
