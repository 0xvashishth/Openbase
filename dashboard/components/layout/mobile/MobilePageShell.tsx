"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

export function MobilePageShell({
  children,
  className,
  ...props
}: React.HTMLAttributes<HTMLElement>) {
  return (
    <main
      className={cn(
        "min-h-0 w-full flex-1 overflow-y-auto bg-background",
        // Mobile-only chrome offsets. `max-md:` keeps desktop identical.
        "max-md:pt-[calc(var(--mobile-topbar-height)+var(--safe-top))]",
        "max-md:pb-[calc(var(--mobile-bottombar-height)+var(--safe-bottom))]",
        className
      )}
      {...props}
    >
      {children}
    </main>
  );
}

export function MobileLayout({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex h-full flex-col bg-background max-md:h-dvh", className)}>
      {children}
    </div>
  );
}
