"use client";

import Link from "next/link";
import { ArrowLeft, FolderKanban, Settings, Users } from "lucide-react";
import { NavLink, NavSection } from "./PlatformSidebar";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

/** Org scope: projects + members + settings. No project DB tools. */
export function OrgSidebar({
  orgId,
  currentPath,
  collapsed = false,
}: {
  orgId: string;
  orgName?: string;
  loading?: boolean;
  currentPath: string;
  collapsed?: boolean;
}) {
  const base = `/orgs/${orgId}`;
  return (
    <div className="flex h-full flex-col">
      <nav
        className={
          collapsed
            ? "flex-1 space-y-4 overflow-y-auto overflow-x-hidden px-2 py-3"
            : "flex-1 space-y-4 overflow-y-auto px-3 py-3"
        }
        aria-label="Organization"
      >
        <div className={collapsed ? "flex justify-center pb-1" : "px-1 pb-1"}>
          {collapsed ? (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Link
                    href="/orgs"
                    aria-label="All organizations"
                    title="All organizations"
                    className="mx-auto flex h-10 w-10 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  >
                    <ArrowLeft className="h-4 w-4" aria-hidden />
                    <span className="sr-only">All organizations</span>
                  </Link>
                </TooltipTrigger>
                <TooltipContent side="right">All organizations</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          ) : (
            <Link
              href="/orgs"
              className="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
            >
              <ArrowLeft className="h-3.5 w-3.5" aria-hidden /> All organizations
            </Link>
          )}
        </div>
        <NavSection label="Organization" collapsed={collapsed}>
          <NavLink href={base} active={currentPath === base} icon={<FolderKanban className="h-3.5 w-3.5" />} collapsed={collapsed}>
            Projects
          </NavLink>
          <NavLink
            href={`${base}/members`}
            active={currentPath.endsWith("/members")}
            icon={<Users className="h-3.5 w-3.5" />}
            collapsed={collapsed}
          >
            Members
          </NavLink>
          <NavLink
            href={`${base}/settings`}
            active={currentPath.endsWith("/settings")}
            icon={<Settings className="h-3.5 w-3.5" />}
            collapsed={collapsed}
          >
            Settings
          </NavLink>
        </NavSection>
      </nav>
    </div>
  );
}
