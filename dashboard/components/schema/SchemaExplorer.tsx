"use client";

import { useEffect, useMemo, useState } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  type Node,
  type Edge,
  type NodeTypes,
  MarkerType,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { EmptyState } from "@/components/ui";
import { SchemaSkeleton } from "@/components/ui/skeletons";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { normalizeFullSchema } from "@/lib/schema";
import type { FullSchema, SchemaInfo } from "@/lib/types";

/** Monochrome table node: uses theme tokens so light/dark both work. */
function TableNode({ data }: { data: Record<string, any> }) {
  const schema = data.schema as SchemaInfo;
  const columns = Array.isArray(schema?.columns) ? schema.columns : [];
  const shown = columns.slice(0, 12);
  const hidden = columns.length - shown.length;

  return (
    <div className="min-w-[190px] max-w-[270px] overflow-hidden rounded-lg border-2 border-border bg-card text-xs shadow-sm">
      <div className="border-b border-border bg-muted px-2.5 py-1.5 font-semibold text-foreground">
        {(schema?.collection as string) ?? (schema as unknown as { name?: string })?.name ?? "unknown"}
      </div>
      <div className="py-1">
        {shown.map((col) => (
          <div key={col.name} className="flex items-center gap-1.5 px-2.5 py-0.5 text-muted-foreground">
            <span className="w-5 text-[10px] font-semibold text-muted-foreground/70">
              {col.is_primary ? "PK" : col.is_unique ? "UQ" : ""}
            </span>
            <span className="truncate font-mono text-[11px] text-foreground">{col.name}</span>
            <span className="ml-auto shrink-0 text-[10px] text-muted-foreground/70">{col.data_type}</span>
          </div>
        ))}
        {columns.length === 0 && (
          <p className="px-2.5 py-1 text-[11px] text-muted-foreground">No columns reported.</p>
        )}
        {hidden > 0 && (
          <p className="px-2.5 py-0.5 text-[10px] text-muted-foreground">+{hidden} more…</p>
        )}
      </div>
    </div>
  );
}

const nodeTypes: NodeTypes = {
  tableNode: TableNode as never,
};

export function layoutGrid(count: number): { perRow: number; xGap: number; yGap: number } {
  return { perRow: Math.max(1, Math.ceil(Math.sqrt(Math.max(1, count)))), xGap: 300, yGap: 220 };
}

export function SchemaExplorer({
  projectId,
  engine,
}: {
  projectId: string;
  engine: string;
}) {
  const [schema, setSchema] = useState<FullSchema | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState("");
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const supportsFK = schema?.capabilities?.supports_foreign_keys ?? false;

  useEffect(() => {
    const token = authToken();
    if (!token) return;
    setLoading(true);
    api
      .getFullSchema(token, projectId)
      .then((s) => {
        setSchema(normalizeFullSchema(s));
        setError(null);
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Failed to load schema"))
      .finally(() => setLoading(false));
  }, [projectId]);

  const collections = useMemo(() => {
    const all = Array.isArray(schema?.collections) ? schema.collections : [];
    const q = filter.trim().toLowerCase();
    if (!q) return all;
    return all.filter((c) => c.collection.toLowerCase().includes(q));
  }, [schema, filter]);

  const relationships = useMemo(
    () => (Array.isArray(schema?.relationships) ? schema.relationships : []),
    [schema]
  );

  useEffect(() => {
    const { perRow, xGap, yGap } = layoutGrid(collections.length);
    const newNodes: Node[] = collections.map((c, i) => ({
      id: `table-${c.collection}`,
      type: "tableNode",
      position: { x: (i % perRow) * xGap, y: Math.floor(i / perRow) * yGap },
      data: { schema: c, engine },
    }));
    // Only draw edges whose endpoints are visible after filtering.
    const visible = new Set(collections.map((c) => `table-${c.collection}`));
    const newEdges: Edge[] = relationships
      .filter((r) => visible.has(`table-${r.from_collection}`) && visible.has(`table-${r.to_collection}`))
      .map((r, i) => ({
        id: `edge-${i}`,
        source: `table-${r.from_collection}`,
        target: `table-${r.to_collection}`,
        type: "smoothstep",
        animated: true,
        style: { strokeWidth: 1.5 },
        markerEnd: { type: MarkerType.ArrowClosed },
        label: `${r.from_column} → ${r.to_column}`,
      }));
    setNodes(newNodes);
    setEdges(newEdges);
  }, [collections, relationships, engine, setNodes, setEdges]);

  if (loading) {
    return <SchemaSkeleton />;
  }

  if (error) {
    return (
      <div role="alert" className="rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
        {error}
      </div>
    );
  }

  const total = Array.isArray(schema?.collections) ? schema.collections.length : 0;
  if (!schema || total === 0) {
    return (
      <EmptyState
        title="No tables found"
        hint="Connect a database and ensure it has tables to visualize the schema."
      />
    );
  }

  const hasRelationships = relationships.length > 0;

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
          Schema Explorer
          <Badge variant="secondary">{total} tables</Badge>
          {hasRelationships && <Badge variant="muted">{relationships.length} relationships</Badge>}
          {!supportsFK && <Badge variant="muted">no FK support</Badge>}
        </h3>
        <Input
          aria-label="Filter tables"
          placeholder="Filter tables…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="w-52"
        />
      </div>

      {!supportsFK && (
        <p className="text-xs text-muted-foreground">
          This engine has no foreign-key support — document/limited model, tables render without relationship lines.
        </p>
      )}
      {!hasRelationships && supportsFK && (
        <p className="text-xs text-muted-foreground">
          No foreign key relationships detected. Relationship lines appear when tables have FK constraints.
        </p>
      )}
      {filter.trim() && collections.length === 0 && (
        <EmptyState title="No tables match" hint={`Nothing matches "${filter.trim()}".`} />
      )}

      <div className="h-[380px] overflow-hidden rounded-lg border border-border bg-background sm:h-[500px]">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          nodeTypes={nodeTypes}
          fitView
          colorMode="system"
          proOptions={{ hideAttribution: true }}
        >
          <Background />
          <Controls />
          <MiniMap />
        </ReactFlow>
      </div>
    </div>
  );
}
