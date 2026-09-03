"use client";

import Link from "next/link";
import {
  ArrowLeft,
  Database,
  Home,
  KeyRound,
  Network,
  Plug,
  Radio,
  Settings,
  SquareTerminal,
  Table2,
  Zap,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { NavLink, NavSection } from "./PlatformSidebar";
import { normalizeTool, projectToolPath } from "./nav";

export interface ProjectSidebarProps {
  orgId: string;
  projectId: string;
  projectName?: string;
  engine?: string | null;
  connected?: boolean;
  supportsTriggers?: boolean;
  supportsRealtime?: boolean;
  hasConnection?: boolean;
  currentPath: string;
  loading?: boolean;
}

/**
 * Project scope sidebar (Supabase-style): visible ONLY inside a project.
 * Tools are grouped; connection-gated + capability-gated items render disabled
 * with an explanatory tooltip instead of a broken page.
 */
export function ProjectSidebar({
  orgId,
  projectId,
  projectName,
  engine,
  connected,
  supportsTriggers,
  supportsRealtime,
  hasConnection,
  currentPath,
  loading,
}: ProjectSidebarProps) {
  const tool = normalizeTool(currentPath.split(`/projects/${projectId}`)[1]?.replace(/^\//, "") || "overview");
  const href = (t: string) => projectToolPath(orgId, projectId, t);
  const isActive = (t: string, aliases: string[] = []) =>
    tool === t || aliases.includes(tool);

  if (loading) {
    return (
      <div className="flex h-full flex-col" role="status" aria-label="Loading project navigation">
        <div className="space-y-2 border-b border-border px-4 py-3">
          <Skeleton className="h-3 w-24" />
          <Skeleton className="h-4 w-36" />
          <div className="flex gap-1.5">
            <Skeleton className="h-5 w-16" />
            <Skeleton className="h-5 w-20" />
          </div>
        </div>
        <div className="flex-1 space-y-4 px-3 py-3">
          {Array.from({ length: 3 }).map((_, s) => (
            <div key={s} className="space-y-1.5">
              <Skeleton className="h-3 w-20" />
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
            </div>
          ))}
        </div>
        <span className="sr-only">Loading project navigation…</span>
      </div>
    );
  }

  const locked = hasConnection === false;
  const lockReason = "Connect a database first (Connection tab)";
  const noTriggers = supportsTriggers === false ? "Engine has no native trigger support" : undefined;
  const noRealtime = supportsRealtime === false ? "Engine has no native realtime support" : undefined;

  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-border px-4 py-3">
        <Link
          href={`/orgs/${orgId}`}
          className="inline-flex items-center gap-1.5 rounded-md px-1 py-0.5 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <ArrowLeft className="h-3.5 w-3.5" aria-hidden /> Back to projects
        </Link>
        <p className="mt-1.5 truncate text-sm font-semibold text-foreground" title={projectName}>
          {projectName ?? "Project"}
        </p>
        <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
          {engine && <Badge variant="secondary">{engine}</Badge>}
          {connected != null && (
            <Badge variant={connected ? "success" : "warning"}>
              {connected ? "connected" : "not connected"}
            </Badge>
          )}
        </div>
      </div>
      <nav className="flex-1 space-y-4 overflow-y-auto px-3 py-3" aria-label="Project tools">
        <NavSection label="Project">
          <NavLink href={href("overview")} active={isActive("overview")} icon={<Home className="h-3.5 w-3.5" />}>
            Overview
          </NavLink>
        </NavSection>
        <NavSection label="Database">
          <NavLink
            href={href("tables")}
            active={isActive("tables", ["data"])}
            icon={<Table2 className="h-3.5 w-3.5" />}
            disabled={locked}
            disabledReason={lockReason}
          >
            Tables
          </NavLink>
          <NavLink
            href={href("schema")}
            active={isActive("schema")}
            icon={<Network className="h-3.5 w-3.5" />}
            disabled={locked}
            disabledReason={lockReason}
          >
            Schema
          </NavLink>
          <NavLink
            href={href("sql")}
            active={isActive("sql")}
            icon={<SquareTerminal className="h-3.5 w-3.5" />}
            disabled={locked}
            disabledReason={lockReason}
          >
            SQL Editor
          </NavLink>
        </NavSection>
        <NavSection label="Backend">
          <NavLink href={href("api")} active={isActive("api", ["api-keys"])} icon={<KeyRound className="h-3.5 w-3.5" />}>
            API Keys
          </NavLink>
          <NavLink
            href={href("functions")}
            active={isActive("functions")}
            icon={<Zap className="h-3.5 w-3.5" />}
            disabled={locked}
            disabledReason={lockReason}
          >
            Functions
          </NavLink>
          <NavLink
            href={href("triggers")}
            active={isActive("triggers")}
            icon={<Database className="h-3.5 w-3.5" />}
            disabled={locked || supportsTriggers === false}
            disabledReason={locked ? lockReason : noTriggers}
          >
            Triggers
          </NavLink>
          <NavLink
            href={href("realtime")}
            active={isActive("realtime")}
            icon={<Radio className="h-3.5 w-3.5" />}
            disabled={locked || supportsRealtime === false}
            disabledReason={locked ? lockReason : noRealtime}
          >
            Realtime
          </NavLink>
        </NavSection>
        <NavSection label="Configure">
          <NavLink href={href("connection")} active={isActive("connection")} icon={<Plug className="h-3.5 w-3.5" />}>
            Connection
          </NavLink>
          <NavLink href={href("settings")} active={isActive("settings")} icon={<Settings className="h-3.5 w-3.5" />}>
            Settings
          </NavLink>
        </NavSection>
      </nav>
    </div>
  );
}
