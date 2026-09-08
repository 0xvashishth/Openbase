"use client";

import * as React from "react";
import { createContext, useContext, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Building2,
  FolderKanban,
  Home,
  Table2,
  Network,
  SquareTerminal,
  Cable,
  KeyRound,
  Zap,
  Database,
  Radio,
  Plug,
  Settings,
  Users,
  Mail,
  UserCircle,
  ArrowLeft,
  LogOut,
  ChevronRight,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { parseRoute, PROJECT_TOOL_META, projectToolPath, type ProjectToolSection } from "../nav";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetClose } from "@/components/ui/sheet";
import { useOrgList } from "../switchers";
import { useAuth } from "@/components/AuthProvider";
import { useProject } from "@/lib/project-context";
import { light, selection, medium } from "@/hooks/useHaptics";

interface MobileSheetContextValue {
  open: boolean;
  setOpen: (open: boolean) => void;
}

const MobileSheetContext = createContext<MobileSheetContextValue | null>(null);

export function MobileSheetProvider({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <MobileSheetContext.Provider value={{ open, setOpen }}>
      {children}
    </MobileSheetContext.Provider>
  );
}

export function useMobileSheet() {
  const context = useContext(MobileSheetContext);
  if (!context) {
    throw new Error("useMobileSheet must be used within a MobileSheetProvider");
  }
  return context;
}

interface NavSectionProps {
  label: string;
  children: React.ReactNode;
}

function NavSection({ label, children }: NavSectionProps) {
  return (
    <div className="space-y-1">
      <p className="px-4 py-2 text-micro font-w510 uppercase tracking-wide text-muted-foreground">{label}</p>
      <div className="space-y-0.5 px-2">{children}</div>
    </div>
  );
}

interface NavItemProps {
  href: string;
  icon: React.ReactNode;
  label: string;
  active?: boolean;
  disabled?: boolean;
  disabledReason?: string;
  onClick?: () => void;
}

function NavItem({ href, icon, label, active, disabled, disabledReason, onClick }: NavItemProps) {
  const { setOpen } = useMobileSheet();

  if (disabled) {
    return (
      <button
        disabled
        aria-disabled="true"
        title={disabledReason}
        className="flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-base font-normal text-muted-foreground/50 cursor-not-allowed"
      >
        <span className="shrink-0" aria-hidden>{icon}</span>
        <span className="truncate">{label}</span>
      </button>
    );
  }

  const handleClick = () => {
    onClick?.();
    setOpen(false);
  };

  return (
    <Link
      href={href}
      prefetch
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-base transition-colors",
        active
          ? "bg-primary/10 text-primary font-w510"
          : "text-foreground hover:bg-accent hover:text-foreground"
      )}
      onClick={handleClick}
    >
      <span className="shrink-0" aria-hidden>{icon}</span>
      <span className="truncate">{label}</span>
      {active && <ChevronRight className="ml-auto h-4 w-4 text-primary" aria-hidden />}
    </Link>
  );
}

const ICONS: Record<string, React.ReactNode> = {
  overview: <Home className="h-5 w-5" />,
  tables: <Table2 className="h-5 w-5" />,
  schema: <Network className="h-5 w-5" />,
  sql: <SquareTerminal className="h-5 w-5" />,
  connect: <Cable className="h-5 w-5" />,
  api: <KeyRound className="h-5 w-5" />,
  functions: <Zap className="h-5 w-5" />,
  triggers: <Database className="h-5 w-5" />,
  realtime: <Radio className="h-5 w-5" />,
  "db-source": <Plug className="h-5 w-5" />,
  settings: <Settings className="h-5 w-5" />,
};

const NEEDS_CONNECTION = new Set(["tables", "schema", "sql", "functions", "triggers", "realtime"]);
const SECTIONS: ProjectToolSection[] = ["Project", "Database", "Backend", "Configure"];

/**
 * Project tool list for the drawer. Rendered only inside project scope, where
 * ProjectProvider is mounted, so connection/capability locks settle the same
 * way the desktop ProjectSidebar does.
 */
function ProjectSheetTools({
  orgId,
  projectId,
  currentPath,
}: {
  orgId: string;
  projectId: string;
  currentPath: string;
}) {
  const { hasConnection, supportsTriggers, supportsRealtime, loading } = useProject();
  const tool = currentPath.split(`/projects/${projectId}`)[1]?.replace(/^\//, "") || "overview";
  const lockReason = "Connect a database first (DB Source tab)";
  return (
    <div className="flex flex-col h-full">
      <div className="px-4 py-3 border-b border-border">
        <Link href={`/orgs/${orgId}`} className="flex items-center gap-2 text-label text-muted-foreground hover:text-foreground">
          <ArrowLeft className="h-5 w-5" aria-hidden />
          <span>Back to projects</span>
        </Link>
      </div>
      {SECTIONS.map((section) => (
        <NavSection key={section} label={section}>
          {PROJECT_TOOL_META.filter((t) => t.section === section).map((t) => {
            const locked = NEEDS_CONNECTION.has(t.slug) && !loading && !hasConnection;
            const disabled =
              locked ||
              (t.slug === "triggers" && !loading && !supportsTriggers) ||
              (t.slug === "realtime" && !loading && !supportsRealtime);
            return (
              <NavItem
                key={t.slug}
                href={projectToolPath(orgId, projectId, t.slug)}
                icon={ICONS[t.slug]}
                label={t.label}
                active={tool === t.slug || (t.aliases ?? []).includes(tool)}
                disabled={disabled}
                disabledReason={disabled ? lockReason : undefined}
              />
            );
          })}
        </NavSection>
      ))}
    </div>
  );
}

function MobileSheetContent() {
  const pathname = usePathname();
  const route = parseRoute(pathname);
  const { user, logout } = useAuth();
  const { open, setOpen } = useMobileSheet();

  const inOrgScope = route.scope === "org" || route.scope === "project";
  const { orgs } = useOrgList(inOrgScope);

  const handleOpenChange = (newOpen: boolean) => {
    setOpen(newOpen);
    if (newOpen) light();
    else selection();
  };

  const renderContent = () => {
    if (route.scope === "platform") {
      return (
        <div className="flex flex-col h-full">
          <NavSection label="Workspace">
            <NavItem href="/orgs" icon={<Building2 className="h-5 w-5" />} label="Organizations" active={pathname === "/orgs" || pathname.startsWith("/orgs/")} />
            <NavItem href="/projects" icon={<FolderKanban className="h-5 w-5" />} label="All projects" active={pathname.startsWith("/projects")} />
            <NavItem href="/settings/email" icon={<Mail className="h-5 w-5" />} label="Email" active={pathname.startsWith("/settings/email")} />
            <NavItem href="/account" icon={<UserCircle className="h-5 w-5" />} label="Account" active={pathname.startsWith("/account")} />
          </NavSection>
        </div>
      );
    }

    if (route.scope === "org" && route.orgId) {
      const base = `/orgs/${route.orgId}`;
      return (
        <div className="flex flex-col h-full">
          <div className="px-4 py-3 border-b border-border">
            <Link href="/orgs" className="flex items-center gap-2 text-label text-muted-foreground hover:text-foreground" onClick={() => handleOpenChange(false)}>
              <ArrowLeft className="h-5 w-5" aria-hidden />
              <span>All organizations</span>
            </Link>
          </div>
          <NavSection label="Organization">
            <NavItem href={base} icon={<FolderKanban className="h-5 w-5" />} label="Projects" active={pathname === base} />
            <NavItem href={`${base}/members`} icon={<Users className="h-5 w-5" />} label="Members" active={pathname.endsWith("/members")} />
            <NavItem href={`${base}/settings`} icon={<Settings className="h-5 w-5" />} label="Settings" active={pathname.endsWith("/settings")} />
          </NavSection>
        </div>
      );
    }

    if (route.scope === "project" && route.orgId && route.projectId) {
      return <ProjectSheetTools orgId={route.orgId} projectId={route.projectId} currentPath={pathname} />;
    }

    return <div className="px-4 py-6 text-center text-muted-foreground">No navigation available</div>;
  };

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent side="left" className="w-mobile-sheet max-w-[85vw] p-0 pb-safe pt-safe md:hidden">
        <div className="flex h-full flex-col">
          <div className="flex h-12 items-center justify-between px-4 border-b border-border">
            <h2 className="text-heading font-w510">Menu</h2>
            <SheetClose asChild>
              <Button variant="ghost" size="icon" className="touch-target" onClick={() => selection()}>
                <span className="sr-only">Close menu</span>
              </Button>
            </SheetClose>
          </div>
          <div className="flex-1 overflow-y-auto">{renderContent()}</div>
          <div className="border-t border-border p-4 space-y-2">
            {user && (
              <>
                <div className="px-3 py-2 text-label text-muted-foreground">{user.email}</div>
                <Button variant="outline" className="w-full justify-start gap-2" onClick={() => { logout(); medium(); }}>
                  <LogOut className="h-4 w-4" aria-hidden /> Sign out
                </Button>
              </>
            )}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}

export function MobileSheet() {
  return <MobileSheetContent />;
}

export function MobileSheetTrigger() {
  const { setOpen } = useMobileSheet();

  return (
    <Button
      variant="ghost"
      size="icon"
      className="touch-target -ml-1"
      onClick={() => { light(); setOpen(true); }}
      aria-label="Open menu"
    >
      <span className="sr-only">Open menu</span>
    </Button>
  );
}