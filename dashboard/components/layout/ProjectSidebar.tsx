"use client";

import Link from "next/link";
import {
  ArrowLeft,
  Cable,
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
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { NavLink, NavSection } from "./PlatformSidebar";
import { normalizeTool, projectToolPath } from "./nav";

export interface ProjectSidebarProps {
  orgId: string;
  projectId: string;
  projectName?: string;
  supportsTriggers?: boolean;
  supportsRealtime?: boolean;
  hasConnection?: boolean;
  currentPath: string;
  loading?: boolean;
  collapsed?: boolean;
}

/**
 * Project scope sidebar (Supabase-style): visible ONLY inside a project.
 * Kept clean on purpose — just back-navigation and the grouped tools.
 * Project name/org context lives in the page header + breadcrumbs,
 * so the sidebar never duplicates it. Connection/engine status lives
 * in the top header, gated on settled data so it never flashes
 * "not connected".
 */
export function ProjectSidebar({
  orgId,
  projectId,
  supportsTriggers,
  supportsRealtime,
  hasConnection,
  currentPath,
  loading,
  collapsed = false,
}: ProjectSidebarProps) {
  const tool = normalizeTool(currentPath.split(`/projects/${projectId}`)[1]?.replace(/^\//, "") || "overview");
  const href = (t: string) => projectToolPath(orgId, projectId, t);
  const isActive = (t: string, aliases: string[] = []) =>
    tool === t || aliases.includes(tool);

  if (loading) {
    if (collapsed) {
      return (
        <div className="flex h-full flex-col items-center gap-2 px-2 py-3" role="status" aria-label="Loading project navigation">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-9 w-9 rounded-md" />
          ))}
          <span className="sr-only">Loading project navigation…</span>
        </div>
      );
    }
    return (
      <div className="flex h-full flex-col" role="status" aria-label="Loading project navigation">
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
  const lockReason = "Connect a database first (DB Source tab)";
  const noTriggers = supportsTriggers === false ? "Engine has no native trigger support" : undefined;
  const noRealtime = supportsRealtime === false ? "Engine has no native realtime support" : undefined;

  return (
    <div className="flex h-full flex-col">
      <nav
        className={
          collapsed
            ? "flex-1 space-y-4 overflow-y-auto overflow-x-hidden px-2 py-3"
            : "flex-1 space-y-4 overflow-y-auto px-3 py-3"
        }
        aria-label="Project tools"
      >
        <div className={collapsed ? "flex justify-center pb-1" : "px-1 pb-1"}>
          {collapsed ? (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Link
                    href={`/orgs/${orgId}`}
                    aria-label="Back to projects"
                    title="Back to projects"
                    className="mx-auto flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  >
                    <ArrowLeft className="h-4 w-4" aria-hidden />
                    <span className="sr-only">Back to projects</span>
                  </Link>
                </TooltipTrigger>
                <TooltipContent side="right">Back to projects</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          ) : (
            <Link
              href={`/orgs/${orgId}`}
              className="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-label font-normal text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
            >
              <ArrowLeft className="h-3.5 w-3.5" aria-hidden /> Back to projects
            </Link>
          )}
        </div>
        <NavSection label="Project" collapsed={collapsed}>
          <NavLink href={href("overview")} active={isActive("overview")} icon={<Home className="h-3.5 w-3.5" />} collapsed={collapsed}>
            Overview
          </NavLink>
        </NavSection>
        <NavSection label="Database" collapsed={collapsed}>
          <NavLink
            href={href("tables")}
            active={isActive("tables", ["data"])}
            icon={<Table2 className="h-3.5 w-3.5" />}
            disabled={locked}
            disabledReason={lockReason}
            collapsed={collapsed}
          >
            Tables
          </NavLink>
          <NavLink
            href={href("schema")}
            active={isActive("schema")}
            icon={<Network className="h-3.5 w-3.5" />}
            disabled={locked}
            disabledReason={lockReason}
            collapsed={collapsed}
          >
            Schema
          </NavLink>
          <NavLink
            href={href("sql")}
            active={isActive("sql")}
            icon={<SquareTerminal className="h-3.5 w-3.5" />}
            disabled={locked}
            disabledReason={lockReason}
            collapsed={collapsed}
          >
            SQL Editor
          </NavLink>
        </NavSection>
        <NavSection label="Backend" collapsed={collapsed}>
          <NavLink href={href("connect")} active={isActive("connect")} icon={<Cable className="h-3.5 w-3.5" />} collapsed={collapsed}>
            Connect
          </NavLink>
          <NavLink href={href("api")} active={isActive("api", ["api-keys"])} icon={<KeyRound className="h-3.5 w-3.5" />} collapsed={collapsed}>
            API Keys
          </NavLink>
          <NavLink
            href={href("functions")}
            active={isActive("functions")}
            icon={<Zap className="h-3.5 w-3.5" />}
            disabled={locked}
            disabledReason={lockReason}
            collapsed={collapsed}
          >
            Functions
          </NavLink>
          <NavLink
            href={href("triggers")}
            active={isActive("triggers")}
            icon={<Database className="h-3.5 w-3.5" />}
            disabled={locked || supportsTriggers === false}
            disabledReason={locked ? lockReason : noTriggers}
            collapsed={collapsed}
          >
            Triggers
          </NavLink>
          <NavLink
            href={href("realtime")}
            active={isActive("realtime")}
            icon={<Radio className="h-3.5 w-3.5" />}
            disabled={locked || supportsRealtime === false}
            disabledReason={locked ? lockReason : noRealtime}
            collapsed={collapsed}
          >
            Realtime
          </NavLink>
        </NavSection>
        <NavSection label="Configure" collapsed={collapsed}>
          <NavLink href={href("db-source")} active={isActive("db-source", ["connection"])} icon={<Plug className="h-3.5 w-3.5" />} collapsed={collapsed}>
            DB Source
          </NavLink>
          <NavLink href={href("settings")} active={isActive("settings")} icon={<Settings className="h-3.5 w-3.5" />} collapsed={collapsed}>
            Settings
          </NavLink>
        </NavSection>
      </nav>
    </div>
  );
}
