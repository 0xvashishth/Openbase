"use client";

import { usePathname } from "next/navigation";
import * as React from "react";
import { Menu, PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { parseRoute } from "./nav";
import { PlatformSidebar } from "./PlatformSidebar";
import { OrgSidebar } from "./OrgSidebar";
import { ProjectSidebar } from "./ProjectSidebar";
import { Topbar } from "./Topbar";
import { Sheet, SheetContent } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { StatusBar } from "./StatusBar";
import { ProjectProvider, useProject } from "@/lib/project-context";
import { cn } from "@/lib/utils";

/**
 * Thin top progress bar shown briefly after every route change so sidebar /
 * button clicks feel interactive while the new page's skeletons load.
 */
function RouteProgress({ currentPath }: { currentPath: string }) {
  const [visible, setVisible] = React.useState(false);
  const first = React.useRef(true);
  React.useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    setVisible(true);
    const t = setTimeout(() => setVisible(false), 600);
    return () => clearTimeout(t);
  }, [currentPath]);
  if (!visible) return null;
  return (
    <div role="status" aria-label="Loading new page" className="h-0.5 w-full overflow-hidden bg-border">
      <div className="h-full w-1/3 animate-[progress-slide_0.6s_ease-in-out_infinite] bg-primary" />
      <span className="sr-only">Loading new page…</span>
    </div>
  );
}

function ProjectSidebarConnected({ currentPath, collapsed = false }: { currentPath: string; collapsed?: boolean }) {
  const { orgId, projectId, project, hasConnection, supportsTriggers, supportsRealtime, loading } =
    useProject();
  return (
    <ProjectSidebar
      orgId={orgId}
      projectId={projectId}
      projectName={project?.name}
      // Gate tool-locking on settled data: while loading, hasConnection stays
      // undefined so nothing renders a premature "not connected" state.
      hasConnection={loading ? undefined : hasConnection}
      supportsTriggers={loading ? undefined : supportsTriggers}
      supportsRealtime={loading ? undefined : supportsRealtime}
      currentPath={currentPath}
      loading={loading && !project}
      collapsed={collapsed}
    />
  );
}

/**
 * Single ProjectProvider wraps BOTH the sidebar and the page content,
 * so tool pages can call useProject() with zero extra fetches.
 */
function ProjectTopbar({ orgName, orgLoading }: { orgName?: string; orgLoading?: boolean }) {
  const { project, engine, hasConnection, loading } = useProject();
  return (
    <Topbar
      orgName={orgName}
      orgLoading={orgLoading}
      projectName={project?.name}
      projectLoading={loading && !project}
      engine={engine}
      // Settled-only: undefined while loading so the header shows skeletons,
      // never a premature "not connected".
      connected={loading ? undefined : hasConnection}
    />
  );
}

function ProjectShell({
  currentPath,
  children,
  orgName,
  orgLoading,
}: {
  currentPath: string;
  children: React.ReactNode;
  orgName?: string;
  orgLoading?: boolean;
}) {
  const route = parseRoute(currentPath);
  if (route.scope !== "project" || !route.orgId || !route.projectId) return <>{children}</>;
  return (
    <ProjectProvider orgId={route.orgId} projectId={route.projectId}>
      <ProjectChrome currentPath={currentPath} orgId={route.orgId} orgName={orgName} orgLoading={orgLoading}>
        {children}
      </ProjectChrome>
    </ProjectProvider>
  );
}

function ProjectChrome({
  currentPath,
  children,
  orgId,
  orgName,
  orgLoading,
}: {
  currentPath: string;
  children: React.ReactNode;
  orgId?: string | null;
  orgName?: string;
  orgLoading?: boolean;
}) {
  const [mobileOpen, setMobileOpen] = React.useState(false);
  const [collapsed, setCollapsed] = React.useState(false);
  React.useEffect(() => setMobileOpen(false), [currentPath]);
  return (
    <div className="flex h-full flex-col">
      <StatusBar orgId={orgId} orgName={orgName} orgLoading={orgLoading} />
      <ProjectTopbar orgName={orgName} orgLoading={orgLoading} />
      <div className="flex min-h-0 flex-1">
        <aside
          className={cn(
            "hidden shrink-0 flex-col border-r border-border bg-card transition-all md:flex",
            collapsed ? "w-16" : "w-60"
          )}
        >
          <div className="flex-1 min-h-0 overflow-hidden">
            <ProjectSidebarConnected currentPath={currentPath} collapsed={collapsed} />
          </div>
          <div className="border-t border-border p-2">
            <button
              onClick={() => setCollapsed((v) => !v)}
              aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
              aria-expanded={!collapsed}
              title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
              className="flex w-full items-center justify-center rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            >
              {collapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
            </button>
          </div>
        </aside>
        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetContent side="left" className="w-72 p-0 md:hidden">
            <ProjectSidebarConnected currentPath={currentPath} collapsed={false} />
          </SheetContent>
        </Sheet>
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="border-b border-border p-2 md:hidden">
            <Button variant="ghost" size="sm" onClick={() => setMobileOpen(true)}>
              <Menu className="h-4 w-4" /> Menu
            </Button>
          </div>
          <main className="flex-1 overflow-y-auto bg-background">{children}</main>
        </div>
      </div>
    </div>
  );
}

function ShellChrome({
  currentPath,
  sidebar,
  orgId,
  orgName,
  orgLoading,
  projectName,
  projectLoading,
  children,
}: {
  currentPath: string;
  sidebar: React.ReactNode;
  orgId?: string | null;
  orgName?: string;
  orgLoading?: boolean;
  projectName?: string;
  projectLoading?: boolean;
  children: React.ReactNode;
}) {
  const [mobileOpen, setMobileOpen] = React.useState(false);
  const [collapsed, setCollapsed] = React.useState(false);

  React.useEffect(() => setMobileOpen(false), [currentPath]);

  // Inject the rail state into whatever sidebar element was provided
  // (PlatformSidebar / OrgSidebar). They accept an optional `collapsed` prop;
  // cloning keeps call sites unchanged so other in-flight work doesn't clash.
  const renderSidebar = (isCollapsed: boolean) =>
    React.isValidElement(sidebar)
      ? React.cloneElement(sidebar as React.ReactElement<{ collapsed?: boolean }>, {
          collapsed: isCollapsed,
        })
      : sidebar;

  return (
    <div className="flex h-full flex-col">
      <StatusBar orgId={orgId} orgName={orgName} orgLoading={orgLoading} />
      <Topbar orgName={orgName} orgLoading={orgLoading} projectName={projectName} projectLoading={projectLoading} />
      <div className="flex min-h-0 flex-1">
        <aside
          className={cn(
            "hidden shrink-0 flex-col border-r border-border bg-card transition-all md:flex",
            collapsed ? "w-16" : "w-60"
          )}
        >
          <div className="flex-1 min-h-0 overflow-hidden">{renderSidebar(collapsed)}</div>
          <div className="border-t border-border p-2">
            <button
              onClick={() => setCollapsed((v) => !v)}
              aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
              aria-expanded={!collapsed}
              title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
              className="flex w-full items-center justify-center rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            >
              {collapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
            </button>
          </div>
        </aside>
        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetContent side="left" className="w-72 p-0 md:hidden">
            {renderSidebar(false)}
          </SheetContent>
        </Sheet>
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="border-b border-border p-2 md:hidden">
            <Button variant="ghost" size="sm" onClick={() => setMobileOpen(true)}>
              <Menu className="h-4 w-4" /> Menu
            </Button>
          </div>
          <main className="flex-1 overflow-y-auto bg-background">{children}</main>
        </div>
      </div>
    </div>
  );
}

function ShellInner({ children, currentPath }: { children: React.ReactNode; currentPath: string }) {
  const route = parseRoute(currentPath);
  const [orgName, setOrgName] = React.useState<string | undefined>(undefined);
  const [orgLoading, setOrgLoading] = React.useState(false);

  React.useEffect(() => {
    const token = authToken();
    if (!token || !route.orgId) {
      setOrgName(undefined);
      setOrgLoading(false);
      return;
    }
    setOrgLoading(true);
    api
      .getOrg(token, route.orgId)
      .then((o) => setOrgName(o.name))
      .catch(() => setOrgName(undefined))
      .finally(() => setOrgLoading(false));
  }, [route.orgId]);

  if (route.scope === "project" && route.orgId && route.projectId) {
    return (
      <ProjectShell currentPath={currentPath} orgName={orgName} orgLoading={orgLoading}>
        {children}
      </ProjectShell>
    );
  }
  if (route.scope === "org" && route.orgId) {
    return (
      <ShellChrome
        currentPath={currentPath}
        sidebar={<OrgSidebar orgId={route.orgId} orgName={orgName} loading={orgLoading && !orgName} currentPath={currentPath} />}
        orgId={route.orgId}
        orgName={orgName}
        orgLoading={orgLoading && !orgName}
      >
        {children}
      </ShellChrome>
    );
  }
  return (
    <ShellChrome
      currentPath={currentPath}
      sidebar={<PlatformSidebar currentPath={currentPath} />}
    >
      {children}
    </ShellChrome>
  );
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname() ?? "/";
  return <ShellInner currentPath={pathname}>{children}</ShellInner>;
}

/** Test-only export: render a specific path without Next router. */
export function AppShellForPath({ path, children }: { path: string; children: React.ReactNode }) {
  return <ShellInner currentPath={path}>{children}</ShellInner>;
}
