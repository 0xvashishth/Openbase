"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Badge, EmptyState, Spinner } from "@/components/ui";
import type { SchemaInfo, ResultSet } from "@/lib/types";

function cellText(v: unknown): string {
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

  const loadCollections = useCallback(() => {
    const token = authToken();
    if (!token) return;
    setLoading(true);
    api
      .listCollections(token, projectId)
      .then((cols) => setCollections(cols.map((c) => c.name)))
      .catch((err) => setError(err instanceof Error ? err.message : "Failed to load collections"))
      .finally(() => setLoading(false));
  }, [projectId]);

  useEffect(() => {
    loadCollections();
  }, [loadCollections]);

  async function selectCollection(name: string) {
    setActive(name);
    setError(null);
    setLoading(true);
    const token = authToken();
    if (!token) return;
    try {
      const [sch, rs] = await Promise.all([
        api.getSchema(token, projectId, name),
        api.queryRows(token, projectId, { collection: name, limit: 100 }),
      ]);
      setSchema(sch);
      setResult(rs);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load table");
    } finally {
      setLoading(false);
    }
  }

  if (error) {
    return (
      <div className="space-y-3">
        <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </div>
        <button onClick={loadCollections} className="text-sm text-brand-600 hover:text-brand-700">
          Retry
        </button>
      </div>
    );
  }

  if (loading && !collections) {
    return (
      <div className="flex items-center gap-2 py-8 text-sm text-slate-500">
        <Spinner className="h-4 w-4" /> Loading tables…
      </div>
    );
  }

  if (!collections || collections.length === 0) {
    return (
      <EmptyState
        title="No tables found"
        hint="Connect a database with tables, or add tables to your connected database."
      />
    );
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[220px_1fr]">
      <aside className="rounded-xl border border-slate-200 bg-white p-2">
        <p className="px-2 pb-1 text-[11px] font-semibold uppercase tracking-wider text-slate-400">
          Tables
        </p>
        <ul className="space-y-0.5">
          {collections.map((name) => (
            <li key={name}>
              <button
                onClick={() => selectCollection(name)}
                className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm ${
                  active === name
                    ? "bg-brand-50 font-medium text-brand-700"
                    : "text-slate-700 hover:bg-slate-100"
                }`}
              >
                <span className="text-slate-400">▤</span>
                {name}
              </button>
            </li>
          ))}
        </ul>
      </aside>

      <section className="overflow-hidden rounded-xl border border-slate-200 bg-white">
        {!active ? (
          <div className="p-6 text-sm text-slate-500">Select a table to view its rows.</div>
        ) : loading ? (
          <div className="flex items-center gap-2 p-6 text-sm text-slate-500">
            <Spinner className="h-4 w-4" /> Loading rows…
          </div>
        ) : (
          <DataSet schema={schema} result={result} />
        )}
      </section>
    </div>
  );
}

function DataSet({ schema, result }: { schema: SchemaInfo | null; result: ResultSet | null }) {
  if (!schema || !result || result.rows.length === 0) {
    return (
      <div className="p-6 text-sm text-slate-500">
        {schema ? `${schema.collection} is empty.` : "No data."}
      </div>
    );
  }

  const columns = schema.columns.length > 0 ? schema.columns.map((c) => c.name) : result.columns;

  return (
    <div>
      <div className="flex items-center gap-2 border-b border-slate-200 px-4 py-2">
        <span className="text-sm font-semibold text-slate-800">{schema.collection}</span>
        <Badge tone="slate">{result.rows.length} rows</Badge>
        <span className="text-xs text-slate-400">showing first 100</span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
            <tr>
              {columns.map((c) => (
                <th key={c} className="border-b border-slate-200 px-4 py-2 font-medium">
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {result.rows.map((row, i) => (
              <tr key={i} className="odd:bg-white even:bg-slate-50/50">
                {columns.map((c) => (
                  <td key={c} className="max-w-[320px] truncate border-b border-slate-100 px-4 py-2 text-slate-700">
                    {cellText(row[c])}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}