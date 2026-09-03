"use client";

import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

function LoadingRegion({
  label = "Loading",
  className,
  children,
}: {
  label?: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div role="status" aria-label={label} aria-busy="true" className={cn("animate-in fade-in duration-200", className)}>
      {children}
      <span className="sr-only">{label}…</span>
    </div>
  );
}

export function CardGridSkeleton({ count = 6, label = "Loading items" }: { count?: number; label?: string }) {
  return (
    <LoadingRegion label={label} className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="rounded-lg border border-border bg-card p-4">
          <div className="flex items-center justify-between gap-2">
            <Skeleton className="h-4 w-2/3" />
            <Skeleton className="h-5 w-16" />
          </div>
          <Skeleton className="mt-2 h-3 w-1/2" />
        </div>
      ))}
    </LoadingRegion>
  );
}

export function ListSkeleton({ rows = 4, label = "Loading" }: { rows?: number; label?: string }) {
  return (
    <LoadingRegion label={label} className="max-w-xl space-y-2">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3">
          <div className="flex-1 space-y-1.5">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-56" />
          </div>
          <Skeleton className="h-8 w-20" />
        </div>
      ))}
    </LoadingRegion>
  );
}

export function FormSkeleton({ label = "Loading form" }: { label?: string }) {
  return (
    <LoadingRegion label={label} className="max-w-xl space-y-3 rounded-xl border border-border bg-card p-4">
      <Skeleton className="h-4 w-32" />
      <Skeleton className="h-9 w-full" />
      <Skeleton className="h-3 w-3/4" />
      <Skeleton className="h-9 w-32" />
    </LoadingRegion>
  );
}

/** Matches the project overview layout: header card + 6 tool tiles. */
export function ProjectOverviewSkeleton() {
  return (
    <LoadingRegion label="Loading project" className="max-w-4xl space-y-4">
      <div className="rounded-lg border border-border bg-card p-6">
        <div className="flex flex-wrap items-center gap-2">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-5 w-20" />
          <Skeleton className="h-5 w-20" />
          <Skeleton className="h-5 w-24" />
        </div>
        <Skeleton className="mt-3 h-4 w-72" />
        <Skeleton className="mt-4 h-9 w-44" />
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="rounded-lg border border-border bg-card p-4">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="mt-2 h-3 w-full" />
            <Skeleton className="mt-1 h-3 w-2/3" />
          </div>
        ))}
      </div>
    </LoadingRegion>
  );
}

/** Generic tool-page skeleton: header card + content rows. */
export function ToolPageSkeleton({ label = "Loading" }: { label?: string }) {
  return (
    <LoadingRegion label={label} className="space-y-4">
      <div className="max-w-3xl space-y-3 rounded-lg border border-border bg-card p-4">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-40" />
      </div>
      <ListSkeleton rows={3} label={label} />
    </LoadingRegion>
  );
}

export function TableBrowserSkeleton({ label = "Loading tables" }: { label?: string }) {
  return (
    <LoadingRegion label={label} className="grid gap-4 lg:grid-cols-[240px_1fr]">
      <div className="rounded-lg border border-border bg-card p-2">
        <Skeleton className="mx-2 mb-2 h-9 w-[calc(100%-16px)]" />
        <div className="space-y-1.5 px-2 py-1">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-7 w-full" />
          ))}
        </div>
      </div>
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <div className="flex items-center gap-2 border-b border-border px-4 py-3">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-5 w-16" />
        </div>
        <div className="space-y-2 p-4">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-8 w-full" />
          ))}
        </div>
      </div>
    </LoadingRegion>
  );
}

export function SchemaSkeleton({ label = "Loading schema" }: { label?: string }) {
  return (
    <LoadingRegion label={label} className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-5 w-16" />
          <Skeleton className="h-5 w-24" />
        </div>
        <Skeleton className="h-9 w-52" />
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="rounded-lg border border-border bg-card p-3">
            <Skeleton className="h-4 w-28" />
            <div className="mt-2 space-y-1.5">
              <Skeleton className="h-3 w-full" />
              <Skeleton className="h-3 w-5/6" />
              <Skeleton className="h-3 w-4/6" />
            </div>
          </div>
        ))}
      </div>
      <Skeleton className="h-[300px] w-full" />
    </LoadingRegion>
  );
}

export function ConnectionSkeleton() {
  return (
    <LoadingRegion label="Loading connection" className="space-y-6">
      <div className="space-y-3">
        <Skeleton className="h-4 w-40" />
        <div className="flex gap-2">
          <Skeleton className="h-8 w-36" />
          <Skeleton className="h-8 w-32" />
        </div>
        <FormSkeleton label="Loading connection form" />
      </div>
      <div className="space-y-2">
        <Skeleton className="h-4 w-40" />
        <Skeleton className="h-16 w-full max-w-xl" />
      </div>
    </LoadingRegion>
  );
}

export function PageHeaderSkeleton() {
  return (
    <div role="status" aria-label="Loading header" className="space-y-2">
      <Skeleton className="h-6 w-48" />
      <Skeleton className="h-4 w-64" />
      <span className="sr-only">Loading…</span>
    </div>
  );
}

export function AppShellSkeleton({ label = "Loading workspace" }: { label?: string }) {
  return (
    <LoadingRegion label={label} className="flex h-full">
      <div className="hidden w-60 shrink-0 flex-col gap-2 border-r border-border bg-background p-3 md:flex">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-3/4" />
      </div>
      <div className="min-w-0 flex-1">
        <Skeleton className="h-14 w-full rounded-none" />
        <div className="mx-auto max-w-6xl space-y-3 px-6 py-6">
          <Skeleton className="h-6 w-48" />
          <Skeleton className="h-32 w-full" />
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
          </div>
        </div>
      </div>
    </LoadingRegion>
  );
}
