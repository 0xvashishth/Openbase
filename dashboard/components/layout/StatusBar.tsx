"use client";

import Link from "next/link";
import { Check, ChevronDown } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { ApiStatusBadge, useApiStatus } from "./ApiStatus";
import { Clock } from "./Clock";
import { useOrgList } from "./switchers";

/**
 * Slim platform status bar (Echoreach-style) pinned above the main header:
 * brand top-left, current org (switchable when 2+ orgs), live API health …
 * and a ticking clock with seconds + timezone on the right.
 */
export function StatusBar({
  orgId,
  orgName,
  orgLoading,
}: {
  orgId?: string | null;
  orgName?: string;
  orgLoading?: boolean;
}) {
  // Single poll shared by both responsive placements below.
  const { health, latency, checks } = useApiStatus();
  const { orgs } = useOrgList(!!orgId);
  const showOrgSwitcher = !!orgId && !!orgs && orgs.length > 1;
  return (
    <div className="flex h-8 shrink-0 items-center justify-between gap-2 border-b border-border bg-muted/40 px-3 text-[11px] sm:px-4">
      <div className="flex min-w-0 items-center gap-2 sm:gap-3">
        <Link
          href="/orgs"
          aria-label="Openbase home"
          className="flex shrink-0 items-center gap-1.5 rounded outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span
            aria-hidden
            className="flex h-5 w-5 items-center justify-center rounded bg-primary text-[10px] font-bold text-primary-foreground"
          >
            O
          </span>
          <span className="font-semibold text-foreground">Openbase</span>
        </Link>
        {orgLoading ? (
          <span role="status" aria-label="Loading organization">
            <Skeleton className="h-4 w-20 rounded-full" />
          </span>
        ) : showOrgSwitcher ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                aria-label="Switch organization"
                className="flex max-w-40 items-center gap-1 rounded-full border border-border bg-secondary px-1.5 py-px text-[11px] font-medium text-secondary-foreground outline-none hover:bg-secondary/80 focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="truncate">{orgName ?? "Organization"}</span>
                <ChevronDown className="h-3 w-3 shrink-0" aria-hidden />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="max-h-64 min-w-44 overflow-y-auto">
              <DropdownMenuLabel>Switch organization</DropdownMenuLabel>
              {orgs!.map((o) => (
                <DropdownMenuItem key={o.id} asChild>
                  <Link href={`/orgs/${o.id}`} aria-current={o.id === orgId ? "page" : undefined} className="flex items-center gap-2">
                    <span className="w-4 shrink-0">
                      {o.id === orgId && <Check className="h-3.5 w-3.5" aria-hidden />}
                    </span>
                    <span className="truncate">{o.name}</span>
                  </Link>
                </DropdownMenuItem>
              ))}
              <DropdownMenuSeparator />
              <DropdownMenuItem asChild>
                <Link href="/orgs">All organizations</Link>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : (
          orgName && (
            <Badge variant="secondary" className="px-1.5 py-px text-[11px]">
              {orgName}
            </Badge>
          )
        )}
        <span className="hidden items-center gap-1.5 min-[560px]:inline-flex">
          <ApiStatusBadge health={health} latency={latency} checks={checks} />
        </span>
      </div>
      <div className="flex shrink-0 items-center gap-2 sm:gap-3">
        <span className="inline-flex items-center gap-1.5 min-[560px]:hidden">
          <ApiStatusBadge health={health} latency={latency} checks={checks} />
        </span>
        <Clock />
      </div>
    </div>
  );
}
