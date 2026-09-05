"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import {
  Building2,
  Cable,
  Database,
  FolderKanban,
  KeyRound,
  Network,
  Plug,
  Radio,
  Search,
  Settings,
  SquareTerminal,
  Table2,
  Zap,
} from "lucide-react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/feedback";
import { parseRoute } from "@/components/layout/nav";
import { cn } from "@/lib/utils";

interface SearchItem {
  id: string;
  group: string;
  label: string;
  hint?: string;
  href: string;
  icon: React.ReactNode;
}

const PROJECT_TOOLS: { slug: string; label: string; icon: React.ReactNode }[] = [
  { slug: "tables", label: "Tables", icon: <Table2 className="h-4 w-4" /> },
  { slug: "schema", label: "Schema", icon: <Network className="h-4 w-4" /> },
  { slug: "sql", label: "SQL Editor", icon: <SquareTerminal className="h-4 w-4" /> },
  { slug: "api", label: "API Keys", icon: <KeyRound className="h-4 w-4" /> },
  { slug: "functions", label: "Functions", icon: <Zap className="h-4 w-4" /> },
  { slug: "triggers", label: "Triggers", icon: <Database className="h-4 w-4" /> },
  { slug: "realtime", label: "Realtime", icon: <Radio className="h-4 w-4" /> },
  { slug: "connect", label: "Connect", icon: <Cable className="h-4 w-4" /> },
  { slug: "db-source", label: "DB Source", icon: <Plug className="h-4 w-4" /> },
  { slug: "settings", label: "Settings", icon: <Settings className="h-4 w-4" /> },
];

const PER_GROUP = 6;

/**
 * Supabase-style global search (⌘K): organizations, projects, tables in the
 * current project, and project tool shortcuts. Fully working — selecting a
 * result navigates to it.
 */
export function GlobalSearch() {
  const pathname = usePathname() ?? "/";
  const router = useRouter();
  const route = parseRoute(pathname);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<SearchItem[]>([]);
  const [active, setActive] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);

  // ⌘K / Ctrl+K toggles from anywhere.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((v) => !v);
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  const load = useCallback(async () => {
    const token = authToken();
    if (!token) return;
    setLoading(true);
    try {
      const orgs = await api.listOrgs(token);
      const found: SearchItem[] = [];
      if (route.scope === "project" && route.orgId && route.projectId) {
        const base = `/orgs/${route.orgId}/projects/${route.projectId}`;
        for (const t of PROJECT_TOOLS) {
          found.push({ id: `tool-${t.slug}`, group: "Tools", label: t.label, hint: "Current project", href: `${base}/${t.slug === "overview" ? "" : t.slug}`, icon: t.icon });
        }
        try {
          const cols = await api.listCollections(token, route.projectId);
          for (const c of cols ?? []) {
            if (c?.name) {
              found.push({
                id: `table-${c.name}`,
                group: "Tables",
                label: c.name,
                hint: "Table in this project",
                href: `${base}/tables`,
                icon: <Table2 className="h-4 w-4" />,
              });
            }
          }
        } catch {
          // tables are best-effort; orgs/projects still work
        }
      }
      for (const o of orgs ?? []) {
        found.push({
          id: `org-${o.id}`,
          group: "Organizations",
          label: o.name,
          hint: o.slug,
          href: `/orgs/${o.id}`,
          icon: <Building2 className="h-4 w-4" />,
        });
      }
      const projectLists = await Promise.all(
        (orgs ?? []).map(async (o) => {
          try {
            const ps = await api.listProjects(token, o.id);
            return { org: o, projects: ps ?? [] };
          } catch {
            return { org: o, projects: [] };
          }
        })
      );
      for (const { org, projects } of projectLists) {
        for (const p of projects) {
          found.push({
            id: `project-${p.id}`,
            group: "Projects",
            label: p.name,
            hint: org.name,
            href: `/orgs/${org.id}/projects/${p.id}`,
            icon: <FolderKanban className="h-4 w-4" />,
          });
        }
      }
      setItems(found);
    } finally {
      setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, pathname]);

  useEffect(() => {
    if (open) {
      setQuery("");
      setActive(0);
      void load();
    }
  }, [open, load]);

  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    const pool = q
      ? items.filter(
          (i) => i.label.toLowerCase().includes(q) || (i.hint ?? "").toLowerCase().includes(q)
        )
      : items;
    const grouped: { group: string; items: SearchItem[] }[] = [];
    for (const item of pool) {
      const g = grouped.find((x) => x.group === item.group);
      if (g) {
        if (g.items.length < PER_GROUP) g.items.push(item);
      } else {
        grouped.push({ group: item.group, items: [item] });
      }
    }
    return grouped;
  }, [items, query]);

  const flat = useMemo(() => results.flatMap((g) => g.items), [results]);

  useEffect(() => setActive(0), [query]);
  useEffect(() => {
    listRef.current
      ?.querySelector(`[data-index="${active}"]`)
      ?.scrollIntoView?.({ block: "nearest" });
  }, [active]);

  const go = useCallback(
    (href: string) => {
      setOpen(false);
      router.push(href);
    },
    [router]
  );

  return (
    <>
      <button
        type="button"
        aria-label="Search (Ctrl+K)"
        onClick={() => setOpen(true)}
        className="hidden h-8 w-full max-w-md items-center gap-2 rounded-md border border-input bg-foreground/[0.02] px-2.5 text-caption text-muted-foreground transition-colors hover:border-ring hover:text-foreground focus-visible:border-ring focus-visible:outline-none sm:flex"
      >
        <Search className="h-4 w-4 shrink-0" aria-hidden />
        <span className="flex-1 truncate text-left">Search organizations, projects, tables…</span>
        <kbd className="hidden shrink-0 rounded-sm border border-border bg-background px-1.5 py-0.5 font-mono text-micro lg:inline-block">
          ⌘K
        </kbd>
      </button>
      <button
        type="button"
        aria-label="Search (Ctrl+K)"
        onClick={() => setOpen(true)}
        className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border bg-transparent text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring sm:hidden"
      >
        <Search className="h-4 w-4" aria-hidden />
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="top-[15%] max-w-xl translate-y-0 gap-0 overflow-hidden p-0 data-[state=closed]:slide-out-to-top-[2%] data-[state=open]:slide-in-from-top-[2%]">
          <DialogTitle className="sr-only">Global search</DialogTitle>
          <div className="flex items-center gap-2 border-b border-border px-3">
            <Search className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden />
            <Input
              autoFocus
              role="combobox"
              aria-expanded
              aria-controls="global-search-results"
              aria-activedescendant={flat[active] ? `search-item-${flat[active].id}` : undefined}
              aria-label="Search organizations, projects and tables"
              placeholder="Search organizations, projects, tables…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  setActive((a) => Math.min(a + 1, flat.length - 1));
                } else if (e.key === "ArrowUp") {
                  e.preventDefault();
                  setActive((a) => Math.max(a - 1, 0));
                } else if (e.key === "Enter" && flat[active]) {
                  e.preventDefault();
                  go(flat[active].href);
                }
              }}
              className="h-12 border-0 bg-transparent text-body-sm focus-visible:border-0 focus-visible:ring-0"
            />
            {loading && <Spinner className="h-4 w-4 shrink-0" />}
          </div>
          <div ref={listRef} id="global-search-results" role="listbox" className="max-h-[50dvh] overflow-y-auto p-2">
            {flat.length === 0 && !loading && (
              <p className="px-3 py-8 text-center text-caption text-muted-foreground">
                {query.trim() ? `No results for "${query.trim()}".` : "Nothing to search yet — create an organization first."}
              </p>
            )}
            {results.map((g) => (
              <div key={g.group} className="mb-1">
                <p className="px-2.5 pb-1 pt-2 text-micro font-w510 uppercase tracking-wide text-muted-foreground">
                  {g.group}
                </p>
                {g.items.map((item) => {
                  const idx = flat.indexOf(item);
                  return (
                    <button
                      key={item.id}
                      id={`search-item-${item.id}`}
                      role="option"
                      aria-selected={idx === active}
                      data-index={idx}
                      onMouseEnter={() => setActive(idx)}
                      onClick={() => go(item.href)}
                      className={cn(
                        "flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left text-caption transition-colors",
                        idx === active ? "bg-accent text-accent-foreground" : "text-muted-foreground"
                      )}
                    >
                      <span className="shrink-0" aria-hidden>{item.icon}</span>
                      <span className="min-w-0 flex-1 truncate font-w510 text-foreground">{item.label}</span>
                      {item.hint && (
                        <>
                          {" "}
                          <span className="shrink-0 truncate font-mono text-micro text-muted-foreground">{item.hint}</span>
                        </>
                      )}
                    </button>
                  );
                })}
              </div>
            ))}
          </div>
          <div className="hidden items-center gap-3 border-t border-border px-4 py-2 text-label text-muted-foreground sm:flex">
            <span><kbd className="rounded-sm border border-border bg-secondary px-1 font-mono text-micro">↑↓</kbd> navigate</span>
            <span><kbd className="rounded-sm border border-border bg-secondary px-1 font-mono text-micro">↵</kbd> open</span>
            <span><kbd className="rounded-sm border border-border bg-secondary px-1 font-mono text-micro">esc</kbd> close</span>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
