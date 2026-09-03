"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ThemeToggle } from "@/components/theme-toggle";
import { useAuth } from "@/components/AuthProvider";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { parseRoute } from "./nav";
import { Skeleton } from "@/components/ui/skeleton";

export function Topbar({
  orgName,
  orgLoading,
  projectName,
  projectLoading,
}: {
  orgName?: string;
  orgLoading?: boolean;
  projectName?: string;
  projectLoading?: boolean;
}) {
  const pathname = usePathname();
  const { user, logout } = useAuth();
  const route = parseRoute(pathname);

  return (
    <header className="flex h-14 shrink-0 items-center justify-between gap-3 border-b border-border bg-background px-4">
      <nav aria-label="Breadcrumb" className="min-w-0">
        <ol className="flex min-w-0 items-center gap-1.5 text-sm">
          {route.scope === "platform" && (
            <li className="truncate font-medium text-foreground">
              {pathname?.startsWith("/projects") ? "All projects" : "Organizations"}
            </li>
          )}
          {route.scope !== "platform" && route.scope !== "auth" && (
            <li className="truncate">
              <Link href="/orgs" className="text-muted-foreground hover:text-foreground">
                Orgs
              </Link>
            </li>
          )}
          {(route.scope === "org" || route.scope === "project") && (
            <>
              <li aria-hidden className="text-muted-foreground">/</li>
              <li className="truncate">
                {route.scope === "project" ? (
                  <Link href={`/orgs/${route.orgId}`} className="text-muted-foreground hover:text-foreground">
                    {orgLoading ? (
                      <span role="status" aria-label="Loading organization" className="inline-flex align-middle">
                        <Skeleton className="h-4 w-24" />
                      </span>
                    ) : (
                      (orgName ?? "Organization")
                    )}
                  </Link>
                ) : orgLoading ? (
                  <span role="status" aria-label="Loading organization">
                    <Skeleton className="h-4 w-24" />
                  </span>
                ) : (
                  <span className="font-medium text-foreground">{orgName ?? "Organization"}</span>
                )}
              </li>
            </>
          )}
          {route.scope === "project" && (
            <>
              <li aria-hidden className="text-muted-foreground">/</li>
              <li className="truncate font-medium text-foreground">
                {projectLoading ? (
                  <span role="status" aria-label="Loading project">
                    <Skeleton className="h-4 w-24" />
                  </span>
                ) : (
                  (projectName ?? "Project")
                )}
              </li>
            </>
          )}
        </ol>
      </nav>
      <div className="flex shrink-0 items-center gap-2">
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
