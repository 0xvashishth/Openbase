"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import * as React from "react";
import { Building2, FolderKanban, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

function SidebarShell({
  children,
  collapsed = false,
}: {
  children: React.ReactNode;
  collapsed?: boolean;
}) {
  return (
    <div className="flex h-full flex-col">
      <nav
        className={collapsed ? "flex-1 space-y-4 overflow-y-auto overflow-x-hidden px-2 py-3" : "flex-1 space-y-4 overflow-y-auto px-3 py-3"}
        aria-label="Primary"
      >
        {children}
      </nav>
    </div>
  );
}

export function NavSection({
  label,
  children,
  collapsed = false,
}: {
  label: string;
  children: React.ReactNode;
  collapsed?: boolean;
}) {
  if (collapsed) {
    // Icon rail: hide the text heading, keep it for screen readers and show
    // a thin divider instead so grouped icons stay visually separated.
    return (
      <div>
        <div aria-hidden="true" className="mx-2 my-1 h-px bg-border" />
        <span className="sr-only">{label}</span>
        <div className="space-y-1">{children}</div>
      </div>
    );
  }
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

function getNavLabel(children: React.ReactNode, label?: string): string {
  if (label) return label;
  if (typeof children === "string") return children;
  if (Array.isArray(children)) {
    return children.filter((c): c is string => typeof c === "string").join(" ").trim();
  }
  return "";
}

export function NavLink({
  href,
  active,
  icon,
  children,
  disabled,
  disabledReason,
  collapsed = false,
  label,
}: {
  href: string;
  active?: boolean;
  icon?: React.ReactNode;
  children: React.ReactNode;
  disabled?: boolean;
  disabledReason?: string;
  collapsed?: boolean;
  label?: string;
}) {
  const pathname = useSafePathname();
  const [pending, setPending] = React.useState(false);

  // Clear the pending spinner once navigation lands.
  React.useEffect(() => {
    setPending(false);
  }, [pathname]);

  const textLabel = getNavLabel(children, label);

  if (collapsed) {
    // Icon rail: icon-only button that stays fully clickable. Hovering shows
    // the full label in a tooltip (side="right") plus native title fallback.
    const railClass = cn(
      "mx-auto flex h-10 w-10 items-center justify-center rounded-md transition-colors",
      active
        ? "bg-accent text-accent-foreground"
        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
      pending && "pointer-events-none opacity-70"
    );
    if (disabled) {
      const tip = disabledReason || textLabel;
      return (
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <span
                aria-disabled="true"
                aria-label={tip}
                title={disabledReason || textLabel}
                className={cn(railClass, "cursor-not-allowed text-muted-foreground/60")}
              >
                {icon && <span aria-hidden>{icon}</span>}
                <span className="sr-only">{children}</span>
              </span>
            </TooltipTrigger>
            {tip && <TooltipContent side="right">{tip}</TooltipContent>}
          </Tooltip>
        </TooltipProvider>
      );
    }
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Link
              href={href}
              prefetch
              aria-current={active ? "page" : undefined}
              aria-disabled={pending || undefined}
              aria-label={pending ? `${textLabel} (loading)` : textLabel}
              title={textLabel}
              onClick={() => {
                if (href !== pathname) setPending(true);
              }}
              className={cn(railClass, "relative")}
            >
              {icon && <span aria-hidden>{icon}</span>}
              <span className="sr-only">{children}</span>
              {pending && (
                <Loader2 className="absolute bottom-0.5 right-0.5 h-3 w-3 animate-spin text-muted-foreground" aria-hidden />
              )}
            </Link>
          </TooltipTrigger>
          {textLabel && <TooltipContent side="right">{textLabel}</TooltipContent>}
        </Tooltip>
      </TooltipProvider>
    );
  }

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
export function PlatformSidebar({ currentPath, collapsed = false }: { currentPath: string; collapsed?: boolean }) {
  return (
    <SidebarShell collapsed={collapsed}>
      <NavSection label="Workspace" collapsed={collapsed}>
        <NavLink href="/orgs" active={currentPath === "/orgs" || currentPath.startsWith("/orgs")} icon={<Building2 className="h-3.5 w-3.5" />} collapsed={collapsed}>
          Organizations
        </NavLink>
        <NavLink href="/projects" active={currentPath.startsWith("/projects")} icon={<FolderKanban className="h-3.5 w-3.5" />} collapsed={collapsed}>
          All projects
        </NavLink>
      </NavSection>
    </SidebarShell>
  );
}
