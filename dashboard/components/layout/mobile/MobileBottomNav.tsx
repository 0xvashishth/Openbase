"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Building2,
  FolderKanban,
  Home,
  Table2,
  SquareTerminal,
  Cable,
  KeyRound,
  Settings,
  Users,
  Mail,
  UserCircle,
  MoreHorizontal,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { parseRoute, projectToolPath } from "../nav";
import { useOrgList, useProjectList } from "../switchers";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { light, selection } from "@/hooks/useHaptics";

interface BottomNavItem {
  label: string;
  icon: React.ReactNode;
  href: string;
  active?: boolean;
  badge?: string;
}

function getPlatformItems(pathname: string, orgs: any[]): BottomNavItem[] {
  return [
    { label: "Orgs", icon: <Building2 className="h-5 w-5" />, href: "/orgs", active: pathname === "/orgs" || pathname.startsWith("/orgs/") },
    { label: "Projects", icon: <FolderKanban className="h-5 w-5" />, href: "/projects", active: pathname.startsWith("/projects") },
    { label: "Settings", icon: <Mail className="h-5 w-5" />, href: "/settings/email", active: pathname.startsWith("/settings/email") },
    { label: "Account", icon: <UserCircle className="h-5 w-5" />, href: "/account", active: pathname.startsWith("/account") },
  ];
}

function getOrgItems(pathname: string, orgId: string, orgName: string, projects: any[]): BottomNavItem[] {
  const base = `/orgs/${orgId}`;
  return [
    { label: "Projects", icon: <FolderKanban className="h-5 w-5" />, href: base, active: pathname === base },
    { label: "Members", icon: <Users className="h-5 w-5" />, href: `${base}/members`, active: pathname.endsWith("/members") },
    { label: "Settings", icon: <Settings className="h-5 w-5" />, href: `${base}/settings`, active: pathname.endsWith("/settings") },
    { label: "Orgs", icon: <Building2 className="h-5 w-5" />, href: "/orgs", active: false },
  ];
}

function getProjectItems(pathname: string, orgId: string, projectId: string, projectName: string): BottomNavItem[] {
  const base = `/orgs/${orgId}/projects/${projectId}`;
  const tool = pathname.split(`/projects/${projectId}`)[1]?.replace(/^\//, "") || "overview";

  const primaryTools = [
    { slug: "overview", label: "Overview", icon: Home, active: tool === "overview" },
    { slug: "tables", label: "Tables", icon: Table2, active: tool === "tables" },
    { slug: "sql", label: "SQL", icon: SquareTerminal, active: tool === "sql" },
    { slug: "connect", label: "Connect", icon: Cable, active: tool === "connect" },
    { slug: "api", label: "API Keys", icon: KeyRound, active: tool === "api" },
  ];

  return primaryTools.map((t) => ({
    label: t.label,
    icon: <t.icon className="h-5 w-5" />,
    href: projectToolPath(orgId, projectId, t.slug),
    active: t.active,
  }));
}

export function MobileBottomNav() {
  const pathname = usePathname();
  const route = parseRoute(pathname);

  const inOrgScope = route.scope === "org" || route.scope === "project";
  const { orgs } = useOrgList(inOrgScope);
  const { projects } = useProjectList(route.orgId, route.scope === "project");

  let items: BottomNavItem[] = [];
  let moreItems: BottomNavItem[] = [];

  if (route.scope === "platform") {
    items = getPlatformItems(pathname, orgs ?? []);
  } else if (route.scope === "org" && route.orgId) {
    const org = orgs?.find((o) => o.id === route.orgId);
    items = getOrgItems(pathname, route.orgId, org?.name ?? "", projects ?? []);
  } else if (route.scope === "project" && route.orgId && route.projectId) {
    const project = projects?.find((p) => p.id === route.projectId);
    const allProjectItems = getProjectItems(pathname, route.orgId, route.projectId, project?.name ?? "");
    items = allProjectItems.slice(0, 4);
    moreItems = allProjectItems.slice(4);
  }

  const visibleItems = items.slice(0, 5);

  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-40 flex h-mobile-bottombar items-center border-t border-border bg-card/95 pb-safe backdrop-blur-sm md:hidden"
      aria-label="Primary navigation"
      role="navigation"
    >
      <div className="flex w-full items-center justify-around px-1">
        {visibleItems.map((item) => (
          <Link
            key={item.href}
            href={item.href}
            className={cn(
              "flex flex-col items-center gap-1 px-3 py-2 text-label touch-target-comfort transition-colors",
              item.active
                ? "text-primary"
                : "text-muted-foreground hover:text-foreground hover:bg-accent rounded-md"
            )}
            aria-current={item.active ? "page" : undefined}
            onClick={() => (item.active ? selection() : light())}
          >
            <span className="relative" aria-hidden>{item.icon}</span>
            <span className="text-caption truncate max-w-[60px]">{item.label}</span>
            {item.badge && (
              <span className="absolute -top-1 -right-1 min-w-[14px] h-4 bg-destructive text-destructive-foreground text-micro font-w510 rounded-full flex items-center justify-center px-1">
                {item.badge}
              </span>
            )}
          </Link>
        ))}

        {moreItems.length > 0 && items.length >= 4 && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="touch-target-comfort flex flex-col items-center gap-1 px-3 py-2 text-label text-muted-foreground hover:text-foreground hover:bg-accent rounded-md"
                onClick={() => light()}
              >
                <MoreHorizontal className="h-5 w-5" aria-hidden />
                <span className="text-caption">More</span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" side="top" className="min-w-[160px]">
              <DropdownMenuLabel>More tools</DropdownMenuLabel>
              <DropdownMenuSeparator />
              {moreItems.map((item) => (
                <DropdownMenuItem key={item.href} asChild>
                  <Link
                    href={item.href}
                    className={cn("flex items-center gap-2", item.active && "text-primary")}
                    onClick={() => (item.active ? selection() : light())}
                  >
                    <span aria-hidden>{item.icon}</span>
                    <span>{item.label}</span>
                  </Link>
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
    </nav>
  );
}