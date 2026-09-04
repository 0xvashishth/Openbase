/**
 * Route-scope helpers for the Supabase-style IA.
 * Pure functions (no Next.js imports) so they are trivially unit-testable.
 */

export type RouteScope = "platform" | "org" | "project" | "auth";

export interface ParsedRoute {
  scope: RouteScope;
  orgId: string | null;
  projectId: string | null;
  tool: string | null;
}

/** Parse a pathname like /orgs/abc/projects/def/tables into scope + ids. */
export function parseRoute(pathname: string | null | undefined): ParsedRoute {
  if (!pathname) return { scope: "platform", orgId: null, projectId: null, tool: null };
  if (pathname.startsWith("/login") || pathname.startsWith("/register")) {
    return { scope: "auth", orgId: null, projectId: null, tool: null };
  }
  const projectMatch = pathname.match(/^\/orgs\/([^/]+)\/projects\/([^/]+)(?:\/(.*))?$/);
  if (projectMatch) {
    return {
      scope: "project",
      orgId: projectMatch[1],
      projectId: projectMatch[2],
      tool: projectMatch[3] || "overview",
    };
  }
  const orgMatch = pathname.match(/^\/orgs\/([^/]+)(?:\/(.*))?$/);
  if (orgMatch) {
    return { scope: "org", orgId: orgMatch[1], projectId: null, tool: orgMatch[2] || null };
  }
  return { scope: "platform", orgId: null, projectId: null, tool: null };
}

export function projectToolPath(orgId: string, projectId: string, tool: string): string {
  const base = `/orgs/${orgId}/projects/${projectId}`;
  if (tool === "overview" || tool === "") return base;
  return `${base}/${tool}`;
}

/** Canonical tool slugs used by the project sidebar. */
export const PROJECT_TOOLS = [
  "overview",
  "tables",
  "data", // legacy alias for tables
  "schema",
  "sql",
  "api",
  "api-keys", // legacy alias for api
  "functions",
  "triggers",
  "realtime",
  "connect",
  "db-source",
  "connection", // legacy alias for db-source
  "settings",
] as const;

export function normalizeTool(tool: string | null): string {
  if (tool === "data") return "tables";
  if (tool === "api-keys") return "api";
  if (tool === "connection") return "db-source";
  if (!tool) return "overview";
  return tool;
}
