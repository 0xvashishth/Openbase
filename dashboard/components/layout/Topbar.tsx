"use client";

import { usePathname } from "next/navigation";
import { ThemeToggle } from "@/components/theme-toggle";
import { useAuth } from "@/components/AuthProvider";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
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
import { Breadcrumbs, toolLabel, type Crumb } from "./Breadcrumbs";
import { GlobalSearch } from "@/components/search/GlobalSearch";
import { parseRoute } from "./nav";
import { useOrgList, useProjectList } from "./switchers";

export function Topbar({
  orgName,
  orgLoading,
  projectName,
  projectLoading,
  engine,
  connected,
}: {
  orgName?: string;
  orgLoading?: boolean;
  projectName?: string;
  projectLoading?: boolean;
  /** Database engine for the current project (project scope only). */
  engine?: string | null;
  /**
   * Settled connection state (project scope only). `undefined` while the
   * project APIs are still in flight — the header renders skeletons instead
   * of a premature "not connected".
   */
  connected?: boolean;
}) {
  const pathname = usePathname();
  const { user, logout } = useAuth();
  const route = parseRoute(pathname);

  // Switcher data: only fetch inside org/project scopes so the platform
  // pages stay light. Hooks no-op gracefully without a token (tests).
  const inOrgScope = route.scope === "org" || route.scope === "project";
  const { orgs } = useOrgList(inOrgScope);
  const { projects } = useProjectList(route.orgId, route.scope === "project");

  // Preserve the current tool when switching projects, e.g. staying on /sql.
  const toolSuffix =
    route.scope === "project" && route.projectId
      ? (pathname?.split(`/projects/${route.projectId}`)[1] ?? "")
      : "";

  const orgDropdown =
    orgs && orgs.length > 1 && route.orgId
      ? orgs.map((o) => ({
          label: o.name,
          href: `/orgs/${o.id}`,
          current: o.id === route.orgId,
        }))
      : undefined;

  const projectDropdown =
    projects && projects.length > 1 && route.orgId && route.projectId
      ? projects.map((p) => ({
          label: p.name,
          href: `/orgs/${route.orgId}/projects/${p.id}${toolSuffix}`,
          current: p.id === route.projectId,
        }))
      : undefined;

  const crumbs: Crumb[] = (() => {
    if (route.scope === "platform") {
      return [{ label: pathname?.startsWith("/projects") ? "All projects" : "Organizations" }];
    }
    if (route.scope === "org" && route.orgId) {
      return [
        { label: "Organizations", href: "/orgs", collapseBelow: "md" },
        {
          label: orgName ?? "Organization",
          loading: orgLoading,
          dropdownItems: orgDropdown,
          dropdownLabel: "Switch organization",
        },
      ];
    }
    if (route.scope === "project" && route.orgId && route.projectId) {
      const base = `/orgs/${route.orgId}/projects/${route.projectId}`;
      const tool = toolLabel(route.tool);
      return [
        { label: "Organizations", href: "/orgs", collapseBelow: "md" },
        {
          label: orgName ?? "Organization",
          href: `/orgs/${route.orgId}`,
          loading: orgLoading,
          collapseBelow: "sm",
          dropdownItems: orgDropdown,
          dropdownLabel: "Switch organization",
        },
        ...(tool === "Overview"
          ? [
              {
                label: projectName ?? "Project",
                loading: projectLoading,
                dropdownItems: projectDropdown,
                dropdownLabel: "Switch project",
              } as Crumb,
            ]
          : [
              {
                label: projectName ?? "Project",
                href: base,
                loading: projectLoading,
                dropdownItems: projectDropdown,
                dropdownLabel: "Switch project",
              } as Crumb,
              { label: tool } as Crumb,
            ]),
      ];
    }
    return [{ label: "Openbase" }];
  })();

  const showBadges = route.scope === "project";
  const badgesSettled = !projectLoading;

  return (
    <header className="flex min-h-14 shrink-0 items-center gap-2 border-b border-border bg-background px-3 py-1.5 sm:gap-3 sm:px-4">
      <div className="flex min-w-0 flex-1 flex-row flex-wrap items-center gap-x-2 gap-y-1">
        <Breadcrumbs items={crumbs} />
        {showBadges && (
          <div className="flex shrink-0 items-center gap-1.5" aria-label="Connection status">
            {!badgesSettled ? (
              <>
                <Skeleton className="h-5 w-16 rounded-full" />
                <Skeleton className="h-5 w-24 rounded-full" />
              </>
            ) : (
              <>
                {engine && <Badge variant="secondary">{engine}</Badge>}
                {connected != null &&
                  (connected ? (
                    <Badge variant="success">connected</Badge>
                  ) : (
                    <Badge variant="warning">not connected</Badge>
                  ))}
              </>
            )}
          </div>
        )}
      </div>
      <div className="flex min-w-0 flex-1 items-center justify-center">
        <GlobalSearch />
      </div>
      <div className="flex shrink-0 items-center gap-1.5 sm:gap-2">
        <ThemeToggle />
        {user && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button aria-label="Account menu" className="rounded-full outline-none focus-visible:ring-2 focus-visible:ring-ring">
                <Avatar className="h-8 w-8">
                  <AvatarFallback>{user.email?.[0]?.toUpperCase() ?? "?"}</AvatarFallback>
                </Avatar>
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuLabel className="max-w-[220px] truncate font-normal text-muted-foreground">
                {user.email}
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => logout()}>Sign out</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
    </header>
  );
}
