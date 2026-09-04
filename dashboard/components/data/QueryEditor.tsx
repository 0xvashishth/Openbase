"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { sql } from "@codemirror/lang-sql";
import { javascript } from "@codemirror/lang-javascript";
import { json } from "@codemirror/lang-json";
import { oneDark } from "@codemirror/theme-one-dark";
import { useTheme } from "next-themes";
import { Play, Trash2 } from "lucide-react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/feedback";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { normalizeResultSet } from "@/lib/schema";
import { editorConfigFor, loadHistory, saveHistory } from "@/lib/query-editor";
import { cellText } from "@/components/data/TableBrowser";
import type { ResultSet } from "@/lib/types";

function extensionsFor(kind: string) {
  switch (kind) {
    case "sql":
    case "arcade":
      return [sql()];
    case "mongo":
      return [javascript()];
    case "vector":
      return [json()];
    default:
      return [];
  }
}

export function QueryEditor({ projectId, engine }: { projectId: string; engine: string | null }) {
  const cfg = editorConfigFor(engine);
  const { resolvedTheme } = useTheme();
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  const [query, setQuery] = useState(cfg.sample);
  const [result, setResult] = useState<ResultSet | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [running, setRunning] = useState(false);
  const [elapsed, setElapsed] = useState<number | null>(null);
  const [history, setHistory] = useState<string[]>([]);

  useEffect(() => {
    setQuery(cfg.sample);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [engine]);

  useEffect(() => {
    setHistory(loadHistory(projectId));
  }, [projectId]);

  const extensions = useMemo(() => extensionsFor(cfg.kind), [cfg.kind]);

  const run = useCallback(async () => {
    const token = authToken();
    if (!token) {
      setError("Not authenticated");
      return;
    }
    if (!query.trim()) {
      setError("Write a query first.");
      return;
    }
    setRunning(true);
    setError(null);
    const started = performance.now();
    try {
      // Every engine supports raw execution now (SQL, mongo-shell, Redis
      // commands, ArcadeDB SQL, Qdrant DSL) — reads plus DML/DDL writes.
      const rs = await api.execSQL(token, projectId, query);
      setResult(normalizeResultSet(rs));
      setElapsed(Math.round(performance.now() - started));
      setHistory(saveHistory(projectId, query));
    } catch (err) {
      setResult(null);
      setElapsed(null);
      setError(err instanceof Error ? err.message : "Query failed");
    } finally {
      setRunning(false);
    }
  }, [query, projectId]);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="secondary">{cfg.languageLabel}</Badge>
        <span className="text-xs text-muted-foreground">{cfg.rawNote}</span>
        {elapsed != null && result && (
          <Badge variant="muted">
            {result.rows.length} rows · {elapsed}ms
          </Badge>
        )}
      </div>

      <div className="overflow-hidden rounded-lg border border-border">
        <CodeMirror
          value={query}
          height="220px"
          extensions={extensions}
          theme={mounted && resolvedTheme === "dark" ? oneDark : "light"}
          onChange={(v) => setQuery(v)}
          placeholder={cfg.placeholder}
          basicSetup={{ lineNumbers: true, highlightActiveLineGutter: true }}
        />
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button onClick={() => void run()} loading={running} disabled={!query.trim()}>
          <Play className="h-4 w-4" aria-hidden /> Run
        </Button>
        <span className="text-xs text-muted-foreground">
          Reads + writes/DDL · max 200 rows · 15s timeout · one statement per run — writes execute immediately
        </span>
      </div>

      {error && (
        <div role="alert" className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      {result ? (
        <QueryResults result={result} />
      ) : (
        !error && <EmptyState title="No results yet" hint="Write a query and press Run." />
      )}

      {history.length > 0 && (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-foreground">History</h3>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                try {
                  localStorage.removeItem(`openbase:sql-history:${projectId}`);
                } catch {}
                setHistory([]);
              }}
            >
              <Trash2 className="h-3.5 w-3.5" aria-hidden /> Clear
            </Button>
          </div>
          <ul className="space-y-1">
            {history.map((h, i) => (
              <li key={`${i}-${h.slice(0, 24)}`}>
                <button
                  onClick={() => setQuery(h)}
                  className="block w-full truncate rounded-md border border-border bg-card px-3 py-1.5 text-left font-mono text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
                  title={h}
                >
                  {h.split("\n")[0].slice(0, 120)}
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function QueryResults({ result }: { result: ResultSet }) {
  const cols = result.columns ?? [];
  const rows = result.rows ?? [];
  if (rows.length === 0) return <EmptyState title="0 rows" hint="The query ran fine — it just matched nothing." />;
  return (
    <div className="overflow-hidden rounded-lg border border-border bg-card">
      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              {cols.map((c) => (
                <TableHead key={c}>{c}</TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row, i) => (
              <TableRow key={i}>
                {cols.map((c) => (
                  <TableCell key={c} className="max-w-[320px] truncate font-mono text-xs">
                    {cellText(row[c])}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
