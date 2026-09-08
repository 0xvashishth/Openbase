"use client";

import * as React from "react";
import { Search } from "lucide-react";
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
import { light } from "@/hooks/useHaptics";
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

  const [searchOpen, setSearchOpen] = React.useState(false);

  return (
    <header className="fixed top-0 left-0 right-0 z-40 flex h-mobile-topbar items-center gap-2 border-b border-border bg-card/80 px-3 pt-safe backdrop-blur-sm md:hidden">
      <MobileSheetTrigger />

      <div className="flex-1 min-w-0 flex items-center gap-2 overflow-hidden">
        <span className="font-w510 text-label truncate">Openbase</span>
        <span className="hidden sm:inline-flex items-center gap-1.5 px-2 py-0.5 bg-accent rounded-badge text-caption text-accent-foreground truncate max-w-[160px]">
          {getContextLabel()}
        </span>
      </div>

      <div className="flex shrink-0 items-center gap-1">
        <Button
          variant="ghost"
          size="icon"
          className="touch-target"
          onClick={() => {
            light();
            setSearchOpen(true);
          }}
          aria-label="Search"
        >
          <Search className="h-5 w-5" aria-hidden />
        </Button>

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

      {searchOpen && (
        <div className="fixed inset-0 z-50 bg-void/90 backdrop-blur-sm flex items-start justify-center pt-20 pb-safe px-4">
          <div className="w-full max-w-md bg-card rounded-lg border border-border overflow-hidden animate-in">
            <div className="flex items-center gap-2 p-4 border-b border-border">
              <GlobalSearch />
              <Button variant="ghost" size="icon" onClick={() => setSearchOpen(false)} aria-label="Close search">
                <Search className="h-5 w-5" aria-hidden />
              </Button>
            </div>
          </div>
        </div>
      )}
    </header>
  );
}