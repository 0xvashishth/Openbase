"use client";

import Link from "next/link";
import { ArrowLeft, FolderKanban, Settings, Users } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { NavLink, NavSection } from "./PlatformSidebar";

function SidebarShell({ orgName, loading, children }: { orgName?: string; loading?: boolean; children: React.ReactNode }) {
  return (
    <div className="flex h-full flex-col">
      <div className="flex h-14 items-center gap-2 border-b border-border px-4">
        <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-sm font-bold text-primary-foreground">
          O
        </div>
        <div className="min-w-0 flex-1 leading-tight">
          {loading ? (
            <div role="status" aria-label="Loading organization">
              <Skeleton className="h-4 w-28" />
              <Skeleton className="mt-1 h-3 w-20" />
              <span className="sr-only">Loading organization…</span>
            </div>
          ) : (
            <>
              <p className="truncate text-sm font-semibold text-foreground">{orgName ?? "Organization"}</p>
              <p className="text-[11px] text-muted-foreground">Organization</p>
            </>
          )}
        </div>
      </div>
      <nav className="flex-1 space-y-4 overflow-y-auto px-3 py-3" aria-label="Organization">
        {children}
      </nav>
    </div>
  );
}

/** Org scope: projects + members + settings. No project DB tools. */
export function OrgSidebar({
  orgId,
  orgName,
  loading,
  currentPath,
}: {
  orgId: string;
  orgName?: string;
  loading?: boolean;
  currentPath: string;
}) {
  const base = `/orgs/${orgId}`;
  return (
    <SidebarShell orgName={orgName} loading={loading}>
      <div className="px-1 pb-1">
        <Link
          href="/orgs"
          className="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <ArrowLeft className="h-3.5 w-3.5" aria-hidden /> All organizations
        </Link>
      </div>
      <NavSection label="Organization">
        <NavLink href={base} active={currentPath === base} icon={<FolderKanban className="h-3.5 w-3.5" />}>
          Projects
        </NavLink>
        <NavLink
          href={`${base}/members`}
          active={currentPath.endsWith("/members")}
          icon={<Users className="h-3.5 w-3.5" />}
        >
          Members
        </NavLink>
        <NavLink
          href={`${base}/settings`}
          active={currentPath.endsWith("/settings")}
          icon={<Settings className="h-3.5 w-3.5" />}
        >
          Settings
        </NavLink>
      </NavSection>
    </SidebarShell>
  );
}
