"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import * as React from "react";
import { Building2, FolderKanban, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

function SidebarShell({
  brandSub,
  children,
}: {
  brandSub: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex h-full flex-col">
      <div className="flex h-14 items-center gap-2 border-b border-border px-4">
        <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-sm font-bold text-primary-foreground">
          O
        </div>
        <div className="min-w-0 leading-tight">
          <p className="text-sm font-semibold text-foreground">Openbase</p>
          <p className="truncate text-[11px] text-muted-foreground">{brandSub}</p>
        </div>
      </div>
      <nav className="flex-1 space-y-4 overflow-y-auto px-3 py-3" aria-label="Primary">
        {children}
      </nav>
    </div>
  );
}

export function NavSection({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <p className="px-2.5 pb-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
        {label}
      </p>
      <div className="space-y-0.5">{children}</div>
    </div>
  );
}

function useSafePathname(): string | null {
  try {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    return usePathname();
  } catch {
    return null;
  }
}

export function NavLink({
  href,
  active,
  icon,
  children,
  disabled,
  disabledReason,
}: {
  href: string;
  active?: boolean;
  icon?: React.ReactNode;
  children: React.ReactNode;
  disabled?: boolean;
  disabledReason?: string;
}) {
  const pathname = useSafePathname();
  const [pending, setPending] = React.useState(false);

  // Clear the pending spinner once navigation lands.
  React.useEffect(() => {
    setPending(false);
  }, [pathname]);

  if (disabled) {
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              aria-disabled="true"
              title={disabledReason}
              className="flex cursor-not-allowed items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm font-medium text-muted-foreground/60"
            >
              {icon && <span className="text-xs" aria-hidden>{icon}</span>}
              {children}
            </span>
          </TooltipTrigger>
          {disabledReason && <TooltipContent>{disabledReason}</TooltipContent>}
        </Tooltip>
      </TooltipProvider>
    );
  }
  return (
    <Link
      href={href}
      prefetch
      aria-current={active ? "page" : undefined}
      aria-disabled={pending || undefined}
      onClick={() => {
        if (href !== pathname) setPending(true);
      }}
      className={cn(
        "flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm font-medium transition-colors",
        active
          ? "bg-accent text-accent-foreground"
          : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
        pending && "pointer-events-none opacity-70"
      )}
    >
      {icon && <span className="text-xs" aria-hidden>{icon}</span>}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {pending && <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" aria-hidden />}
    </Link>
  );
}

/** Platform scope: visible when NO org/project is selected. No DB tools here. */
export function PlatformSidebar({ currentPath }: { currentPath: string }) {
  return (
    <SidebarShell brandSub="Platform">
      <NavSection label="Workspace">
        <NavLink href="/orgs" active={currentPath === "/orgs" || currentPath.startsWith("/orgs")} icon={<Building2 className="h-3.5 w-3.5" />}>
          Organizations
        </NavLink>
        <NavLink href="/projects" active={currentPath.startsWith("/projects")} icon={<FolderKanban className="h-3.5 w-3.5" />}>
          All projects
        </NavLink>
      </NavSection>
    </SidebarShell>
  );
}
