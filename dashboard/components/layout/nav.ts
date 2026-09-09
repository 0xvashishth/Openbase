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

/** A project tool section in the sidebar. */
export type ProjectToolSection = "Project" | "Database" | "Backend" | "Configure";

export interface ProjectToolMeta {
  /** Canonical slug used in URLs. */
  slug: string;
  /** Human label shown in nav, search and tiles. */
  label: string;
  /** One-line description for breadcrumbs info + overview tiles. */
  description: string;
  /** Sidebar grouping. */
  section: ProjectToolSection;
  /** Legacy slugs that redirect to this tool (old bookmarks). */
  aliases?: string[];
}

/**
 * SINGLE SOURCE OF TRUTH for the project tool list. Adding a tool means
 * adding one entry here (plus its icon where presentation needs it):
 * sidebar labels/sections/aliases, breadcrumb labels/descriptions, global
 * search shortcuts and overview tiles all derive from this table.
 */
export const PROJECT_TOOL_META: ProjectToolMeta[] = [
  { slug: "overview", label: "Overview", description: "Project status and shortcuts.", section: "Project" },
  { slug: "tables", label: "Tables", description: "Browse collections and rows — like Supabase's table editor.", section: "Database", aliases: ["data"] },
  { slug: "schema", label: "Schema", description: "Visual tables and relationships.", section: "Database" },
  { slug: "sql", label: "SQL Editor", description: "Run reads and writes in your database's own language.", section: "Database" },
  { slug: "connect", label: "Connect", description: "Connect your app to Openbase — URLs, keys and code snippets.", section: "Backend" },
  { slug: "auth", label: "Authentication", description: "End users, sign-in providers and auth hooks.", section: "Backend" },
  { slug: "api", label: "API Keys", description: "Keys for your auto-generated REST API.", section: "Backend", aliases: ["api-keys"] },
  { slug: "functions", label: "Functions", description: "Serverless functions triggered by data events.", section: "Backend" },
  { slug: "triggers", label: "Triggers", description: "When X happens on a table, run a function or webhook.", section: "Backend" },
  { slug: "realtime", label: "Realtime", description: "Live WebSocket updates for native engines.", section: "Backend" },
  { slug: "db-source", label: "DB Source", description: "Provision a database or connect your own.", section: "Configure", aliases: ["connection"] },
  { slug: "settings", label: "Settings", description: "Project metadata and danger zone.", section: "Configure" },
];

/** Canonical tool slugs used by the project sidebar (derived from META). */
export const PROJECT_TOOLS: readonly string[] = PROJECT_TOOL_META.flatMap((t) =>
  t.aliases ? [t.slug, ...t.aliases] : [t.slug]
);

/** Map of every routable slug (canonical + legacy) to its canonical tool. */
const CANONICAL_BY_SLUG: Record<string, string> = Object.fromEntries(
  PROJECT_TOOL_META.flatMap((t) => [
    [t.slug, t.slug],
    ...(t.aliases ?? []).map((a): [string, string] => [a, t.slug]),
  ])
);

export function normalizeTool(tool: string | null): string {
  if (!tool) return "overview";
  return CANONICAL_BY_SLUG[tool] ?? tool;
}
