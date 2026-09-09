"use client";

import * as React from "react";
import { usePathname } from "next/navigation";
import { parseRoute } from "../nav";
import { useOrgList, useProjectList } from "../switchers";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ThemeToggle } from "@/components/theme-toggle";
import { useAuth } from "@/components/AuthProvider";
import { GlobalSearch } from "@/components/search/GlobalSearch";
import { MobileSheetTrigger } from "./MobileSheet";

export function MobileTopbar() {
  const pathname = usePathname();
  const { user, logout } = useAuth();
  const route = parseRoute(pathname);

  const inOrgScope = route.scope === "org" || route.scope === "project";
  const { orgs } = useOrgList(inOrgScope);
  const { projects } = useProjectList(route.orgId, route.scope === "project");

  const getContextLabel = () => {
    if (route.scope === "platform") return "Organizations";
    if (route.scope === "org" && route.orgId) return orgs?.find((o) => o.id === route.orgId)?.name ?? "Organization";
    if (route.scope === "project" && route.orgId && route.projectId)
      return projects?.find((p) => p.id === route.projectId)?.name ?? "Project";
    return "Openbase";
  };

  return (
    <header className="fixed top-0 left-0 right-0 z-40 flex h-mobile-topbar items-center gap-2 border-b border-border bg-card/80 px-3 pt-safe backdrop-blur-sm md:hidden">
      <MobileSheetTrigger />

      <div className="flex-1 min-w-0 flex items-center gap-2 overflow-hidden">
        <span
          aria-hidden
          className="flex h-6 w-6 shrink-0 items-center justify-center rounded-sm bg-foreground-strong text-label font-w590 text-background"
        >
          O
        </span>
        <span className="font-w510 text-body-sm text-foreground-strong truncate">Openbase</span>
        <span className="hidden sm:inline-flex items-center gap-1.5 px-2 py-0.5 bg-accent rounded-badge text-caption text-accent-foreground truncate max-w-[160px]">
          {getContextLabel()}
        </span>
      </div>

      <div className="flex shrink-0 items-center gap-1">
        <GlobalSearch />

        <ThemeToggle />

        {user && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" className="touch-target -mr-1" aria-label="Account menu">
                <Avatar className="h-8 w-8">
                  <AvatarFallback>{user.email?.[0]?.toUpperCase() ?? "?"}</AvatarFallback>
                </Avatar>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-[200px]">
              <DropdownMenuLabel className="max-w-[220px] truncate">{user.email}</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => logout()}>Sign out</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>

    </header>
  );
}
