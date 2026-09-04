"use client";

import * as React from "react";
import { Info } from "lucide-react";
import { Skeleton as ShadcnSkeleton } from "@/components/ui/skeleton";
import { Separator as ShadcnSeparator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

export function Spinner({ className = "" }: { className?: string }) {
  return (
    <svg
      className={`animate-spin ${className}`}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden
      role="status"
    >
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
    </svg>
  );
}

export function Skeleton({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <ShadcnSkeleton className={className} {...props} />;
}

export function Separator({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <ShadcnSeparator className={className} {...props} />;
}

export function EmptyState({
  title,
  hint,
  action,
}: {
  title: string;
  hint?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border bg-muted/30 px-6 py-12 text-center">
      <p className="text-sm font-medium text-foreground">{title}</p>
      {hint && <p className="mt-1 max-w-sm text-xs text-muted-foreground">{hint}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

export function ErrorBanner({ message }: { message: string }) {
  return (
    <div role="alert" className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
      {message}
    </div>
  );
}

export function SuccessBanner({ message }: { message: string }) {
  return (
    <div role="status" className="rounded-md border border-success/30 bg-success/10 px-3 py-2 text-sm text-success">
      {message}
    </div>
  );
}

export function PageHeader({ title, subtitle, info }: { title: string; subtitle?: string; info?: string }) {
  const tooltipText = info ?? subtitle;
  return (
    <header className="border-b border-border bg-background px-4 py-4 sm:px-6 sm:py-5">
      <h1 className="flex min-w-0 items-center gap-1.5 truncate text-lg font-semibold tracking-tight text-foreground">
        <span className="truncate">{title}</span>
        {tooltipText && (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  aria-label={`About ${title}`}
                  className="shrink-0 rounded text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <Info className="h-4 w-4" aria-hidden />
                </button>
              </TooltipTrigger>
              <TooltipContent side="right" className="max-w-xs">
                {tooltipText}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        )}
      </h1>
    </header>
  );
}
