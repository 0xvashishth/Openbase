"use client";

import Link from "next/link";
import { Fragment } from "react";
import { Check, ChevronDown, ChevronRight, Info } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { normalizeTool } from "./nav";

const TOOL_LABELS: Record<string, string> = {
  overview: "Overview",
  tables: "Tables",
  schema: "Schema",
  sql: "SQL Editor",
  api: "API Keys",
  functions: "Functions",
  triggers: "Triggers",
  realtime: "Realtime",
  connection: "Connection",
  settings: "Settings",
  members: "Members",
};

export function toolLabel(tool: string | null | undefined): string {
  if (!tool) return "Overview";
  const t = normalizeTool(tool);
  if (TOOL_LABELS[t]) return TOOL_LABELS[t];
  return t.charAt(0).toUpperCase() + t.slice(1);
}

/**
 * One-line page descriptions, previously rendered as per-page title headers
 * with an info icon. Those headers duplicated the breadcrumb trail, so the
 * description now lives behind a trailing info icon in the breadcrumbs.
 */
const TOOL_DESCRIPTIONS: Record<string, string> = {
  overview: "Project status and shortcuts.",
  tables: "Browse collections and rows — like Supabase's table editor.",
  schema: "Visual tables and relationships.",
  sql: "Run read-only queries with syntax highlighting.",
  api: "Keys for your auto-generated REST API.",
  functions: "Serverless functions triggered by data events.",
  triggers: "When X happens on a table, run a function or webhook.",
  realtime: "Live WebSocket updates for native engines.",
  connection: "Provision a database or connect your own.",
  settings: "Project metadata and danger zone.",
  members: "People with access to this organization.",
};

/** Description for a project tool slug (handles legacy aliases). */
export function toolDescription(tool: string | null | undefined): string | undefined {
  if (!tool) return TOOL_DESCRIPTIONS.overview;
  return TOOL_DESCRIPTIONS[normalizeTool(tool)];
}

export interface CrumbDropdownItem {
  label: string;
  href: string;
  current?: boolean;
}

export interface Crumb {
  label: string;
  href?: string;
  loading?: boolean;
  /** hide on narrow screens (middle segments collapse first) */
  collapseBelow?: "sm" | "md";
  /**
   * When provided with 2+ entries, a small chevron button appears next to
   * the crumb and opens a shadcn dropdown to switch orgs/projects.
   */
  dropdownItems?: CrumbDropdownItem[];
  /** Accessible label for the switcher trigger, e.g. "Switch organization". */
  dropdownLabel?: string;
}

/**
 * Supabase-style breadcrumb: chevron-separated ancestors as links,
 * current page as plain text with aria-current. Middle segments collapse
 * on small screens so the trail never overflows the header.
 * An optional `info` description renders as a trailing info icon (the
 * per-page title headers were removed as redundant with this trail).
 */
export function Breadcrumbs({ items, info }: { items: Crumb[]; info?: string }) {
  const currentLabel = items.length > 0 ? items[items.length - 1].label : "page";
  return (
    <nav aria-label="Breadcrumb" className="min-w-0">
      <div className="flex min-w-0 items-center gap-0.5">
      <ol className="flex min-w-0 items-center gap-1 text-sm">
        {items.map((item, i) => {
          const last = i === items.length - 1;
          return (
            <Fragment key={`${item.label}-${i}`}>
              {i > 0 && (
                <li
                  aria-hidden
                  className={cn(
                    "shrink-0 text-muted-foreground/60",
                    item.collapseBelow === "sm" && "hidden sm:block",
                    item.collapseBelow === "md" && "hidden md:block"
                  )}
                >
                  <ChevronRight className="h-3.5 w-3.5" />
                </li>
              )}
              <li
                className={cn(
                  "flex min-w-0 items-center gap-0.5 truncate",
                  item.collapseBelow === "sm" && "hidden sm:flex",
                  item.collapseBelow === "md" && "hidden md:flex"
                )}
              >
                {item.loading ? (
                  <span role="status" aria-label={`Loading ${item.label}`}>
                    <Skeleton className="h-4 w-20 sm:w-24" />
                  </span>
                ) : last || !item.href ? (
                  <span aria-current={last ? "page" : undefined} className="truncate font-medium text-foreground">
                    {item.label}
                  </span>
                ) : (
                  <Link
                    href={item.href}
                    className="truncate text-muted-foreground transition-colors hover:text-foreground"
                  >
                    {item.label}
                  </Link>
                )}
                {item.dropdownItems && item.dropdownItems.length > 1 && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <button
                        type="button"
                        aria-label={item.dropdownLabel ?? `Switch ${item.label}`}
                        className="shrink-0 rounded p-0.5 text-muted-foreground outline-none hover:bg-accent hover:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring"
                      >
                        <ChevronDown className="h-3.5 w-3.5" aria-hidden />
                      </button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="start" className="max-h-64 min-w-44 overflow-y-auto">
                      <DropdownMenuLabel>{item.dropdownLabel ?? `Switch ${item.label}`}</DropdownMenuLabel>
                      {item.dropdownItems.map((opt) => (
                        <DropdownMenuItem key={opt.href} asChild>
                          <Link href={opt.href} aria-current={opt.current ? "page" : undefined} className="flex items-center gap-2">
                            <span className="w-4 shrink-0">
                              {opt.current && <Check className="h-3.5 w-3.5" aria-hidden />}
                            </span>
                            <span className="truncate">{opt.label}</span>
                          </Link>
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </li>
            </Fragment>
          );
        })}
      </ol>
      {info && (
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={`About ${currentLabel}`}
                className="shrink-0 rounded p-0.5 text-muted-foreground outline-none hover:bg-accent hover:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring"
              >
                <Info className="h-3.5 w-3.5" aria-hidden />
              </button>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="max-w-xs">
              {info}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}
      </div>
    </nav>
  );
}
