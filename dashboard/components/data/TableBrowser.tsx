"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ArrowDown, ArrowUp, ChevronLeft, ChevronRight } from "lucide-react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { EmptyState } from "@/components/ui/feedback";
import { Skeleton } from "@/components/ui/skeleton";
import { TableBrowserSkeleton } from "@/components/ui/skeletons";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { cn } from "@/lib/utils";
import { normalizeNames, normalizeResultSet, normalizeSchemaInfo } from "@/lib/schema";
import type { SchemaInfo, ResultSet } from "@/lib/types";

const PAGE_SIZE = 25;

export const OPERATORS = [
  { value: "eq", label: "=" },
  { value: "neq", label: "≠" },
  { value: "gt", label: ">" },
  { value: "lt", label: "<" },
  { value: "gte", label: "≥" },
  { value: "lte", label: "≤" },
  { value: "contains", label: "contains" },
] as const;

export function cellText(v: unknown): string {
  if (v === null || v === undefined) return "NULL";
  if (typeof v === "object") {
    try {
      return JSON.stringify(v);
    } catch {
      return String(v);
    }
  }
  return String(v);
}

export function TableBrowser({ projectId }: { projectId: string }) {
  const [collections, setCollections] = useState<string[] | null>(null);
  const [active, setActive] = useState<string | null>(null);
  const [schema, setSchema] = useState<SchemaInfo | null>(null);
  const [result, setResult] = useState<ResultSet | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [query, setQuery] = useState("");
  // Query options.
  const [page, setPage] = useState(0);
  const [orderBy, setOrderBy] = useState<{ field: string; desc: boolean } | null>(null);
  const [filterField, setFilterField] = useState("");
  const [filterOp, setFilterOp] = useState<string>("eq");
  const [filterValue, setFilterValue] = useState("");

  const loadCollections = useCallback(() => {
    const token = authToken();
    if (!token) return;
    setLoading(true);
    api
      .listCollections(token, projectId)
      .then((cols) => setCollections(normalizeNames(cols)))
      .catch((err) => setError(err instanceof Error ? err.message : "Failed to load collections"))
      .finally(() => setLoading(false));
  }, [projectId]);

  useEffect(() => {
    loadCollections();
  }, [loadCollections]);

  const fetchRows = useCallback(
    async (name: string, opts?: { page?: number; order?: typeof orderBy; useFilter?: boolean }) => {
      const token = authToken();
      if (!token) return;
      setLoading(true);
      setError(null);
      try {
        const p = opts?.page ?? page;
        const o = opts?.order !== undefined ? opts.order : orderBy;
        const conditions =
          (opts?.useFilter ?? true) && filterField.trim()
            ? [{ field: filterField.trim(), operator: filterOp, value: coerceValue(filterValue) }]
            : [];
        const [sch, rs] = await Promise.all([
          api.getSchema(token, projectId, name),
          api.queryRows(token, projectId, {
            collection: name,
            limit: PAGE_SIZE,
            offset: p * PAGE_SIZE,
            conditions,
            order_by: o ? [{ field: o.field, desc: o.desc }] : [],
          }),
        ]);
        setSchema(normalizeSchemaInfo(sch));
        setResult(normalizeResultSet(rs));
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load table");
      } finally {
        setLoading(false);
      }
    },
    [projectId, page, orderBy, filterField, filterOp, filterValue]
  );

  async function selectCollection(name: string) {
    setActive(name);
    setPage(0);
    setOrderBy(null);
    setFilterField("");
    setFilterValue("");
    await fetchRows(name, { page: 0, order: null, useFilter: false });
  }

  function toggleSort(col: string) {
    if (!active) return;
    const next = orderBy?.field === col ? { field: col, desc: !orderBy.desc } : { field: col, desc: false };
    setOrderBy(next);
    void fetchRows(active, { order: next });
  }

  function changePage(delta: number) {
    if (!active) return;
    const next = Math.max(0, page + delta);
    setPage(next);
    void fetchRows(active, { page: next });
  }

  const visibleCollections = useMemo(() => {
    const all = collections ?? [];
    const q = query.trim().toLowerCase();
    if (!q) return all;
    return all.filter((n) => n.toLowerCase().includes(q));
  }, [collections, query]);

  if (error && !active) {
    return (
      <div className="space-y-3">
        <div role="alert" className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-caption text-destructive">
          {error}
        </div>
        <Button variant="outline" size="sm" onClick={loadCollections}>
          Retry
        </Button>
      </div>
    );
  }

  if (loading && !collections) {
    return <TableBrowserSkeleton />;
  }

  if (!collections || collections.length === 0) {
    return (
      <EmptyState
        title="No tables found"
        hint="Connect a database with tables, or add tables to your connected database."
      />
    );
  }

  const rows = result?.rows ?? [];
  const schemaCols = schema?.columns ?? [];
  const columns = schemaCols.length > 0 ? schemaCols.map((c) => c.name) : (result?.columns ?? []);

  return (
    <div className="grid gap-4 lg:grid-cols-[240px_1fr]">
      <aside className="rounded-lg border border-border bg-card p-2">
        <div className="px-2 pb-2">
          <Input aria-label="Filter tables" placeholder="Filter tables…" value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
        <p className="px-2 pb-1 text-micro font-w510 uppercase tracking-wide text-muted-foreground">
          Tables ({visibleCollections.length})
        </p>
        <ul className="max-h-[480px] space-y-0.5 overflow-y-auto">
          {visibleCollections.map((name) => (
            <li key={name}>
              <button
                onClick={() => void selectCollection(name)}
                aria-current={active === name ? "true" : undefined}
                className={cn(
                  "flex w-full items-center gap-2 rounded-md border-l-2 border-transparent px-2 py-1.5 text-left text-caption transition-colors",
                  active === name
                    ? "border-primary bg-primary/10 font-w510 text-foreground-strong"
                    : "font-normal text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                )}
              >
                <span aria-hidden>▤</span>
                <span className="truncate">{name}</span>
              </button>
            </li>
          ))}
          {visibleCollections.length === 0 && (
            <li className="px-2 py-4 text-label text-muted-foreground">No tables match.</li>
          )}
        </ul>
      </aside>

      <section className="overflow-hidden rounded-lg border border-border bg-card" aria-busy={loading || undefined}>
        {!active ? (
          <div className="p-6 text-caption text-muted-foreground">Select a table to view its rows.</div>
        ) : loading && rows.length === 0 && !schema ? (
          <div role="status" aria-label="Loading rows" className="space-y-2 p-4">
            <div className="flex items-center gap-2">
              <Skeleton className="h-4 w-32" />
              <Skeleton className="h-5 w-16" />
            </div>
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
            <span className="sr-only">Loading rows…</span>
          </div>
        ) : (
          <div>
            <div className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-2">
              <span className="text-caption font-w510 text-foreground-strong">{schema?.collection ?? active}</span>
              <Badge variant="secondary">{rows.length} rows</Badge>
              <span className="font-mono text-label text-muted-foreground">page {page + 1} · {PAGE_SIZE}/page</span>
              {loading && (
                <span role="status" aria-label="Refreshing rows" className="ml-auto flex items-center gap-1.5 text-label text-muted-foreground">
                  <span className="h-3 w-3 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-muted-foreground" aria-hidden />
                  Refreshing…
                </span>
              )}
            </div>
            {error && (
              <div role="alert" className="mx-4 mt-3 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-caption text-destructive">
                {error}
              </div>
            )}
            <form
              className="flex flex-wrap items-end gap-2 border-b border-border px-4 py-3"
              onSubmit={(e) => {
                e.preventDefault();
                setPage(0);
                if (active) void fetchRows(active, { page: 0 });
              }}
            >
              <div>
                <Label htmlFor="tb-field">Field</Label>
                <Input id="tb-field" placeholder="e.g. status" value={filterField} onChange={(e) => setFilterField(e.target.value)} className="w-36" />
              </div>
              <div>
                <Label htmlFor="tb-op">Op</Label>
                <select
                  id="tb-op"
                  aria-label="Operator"
                  value={filterOp}
                  onChange={(e) => setFilterOp(e.target.value)}
                  className="h-8 rounded-md border border-input bg-foreground/[0.02] px-2 text-caption text-foreground transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-0"
                >
                  {OPERATORS.map((o) => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                </select>
              </div>
              <div>
                <Label htmlFor="tb-value">Value</Label>
                <Input id="tb-value" placeholder="e.g. active" value={filterValue} onChange={(e) => setFilterValue(e.target.value)} className="w-36" />
              </div>
              <Button type="submit" size="sm">Apply</Button>
              {(filterField || orderBy) && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setFilterField("");
                    setFilterValue("");
                    setOrderBy(null);
                    setPage(0);
                    if (active) void fetchRows(active, { page: 0, order: null, useFilter: false });
                  }}
                >
                  Reset
                </Button>
              )}
            </form>
            {rows.length === 0 ? (
              <div className="p-6 text-caption text-muted-foreground">
                {schema ? `${schema.collection} is empty for this filter/page.` : "No data."}
              </div>
            ) : (
              <>
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        {columns.map((c) => (
                          <TableHead key={c}>
                            <button
                              onClick={() => toggleSort(c)}
                              aria-label={`Sort by ${c}`}
                              className="inline-flex items-center gap-1 hover:text-foreground"
                            >
                              {c}
                              {orderBy?.field === c &&
                                (orderBy.desc ? <ArrowDown className="h-3 w-3" /> : <ArrowUp className="h-3 w-3" />)}
                            </button>
                          </TableHead>
                        ))}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {rows.map((row, i) => (
                        <TableRow key={i}>
                          {columns.map((c) => (
                            <TableCell key={c} className="max-w-[320px] truncate font-mono text-label">
                              {cellText(row[c])}
                            </TableCell>
                          ))}
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
                <div className="flex items-center justify-between border-t border-border px-4 py-2">
                  <Button variant="outline" size="sm" disabled={page === 0 || loading} onClick={() => changePage(-1)}>
                    <ChevronLeft className="h-3.5 w-3.5" /> Prev
                  </Button>
                  <span className="font-mono text-label text-muted-foreground">Page {page + 1}</span>
                  <Button variant="outline" size="sm" disabled={rows.length < PAGE_SIZE || loading} onClick={() => changePage(1)}>
                    Next <ChevronRight className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </>
            )}
          </div>
        )}
      </section>
    </div>
  );
}

function coerceValue(raw: string): unknown {
  const t = raw.trim();
  if (t === "") return "";
  if (t === "null") return null;
  if (t === "true") return true;
  if (t === "false") return false;
  const n = Number(t);
  if (t !== "" && !Number.isNaN(n)) return n;
  return raw;
}
