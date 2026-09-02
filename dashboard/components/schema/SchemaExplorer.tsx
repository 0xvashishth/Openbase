"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
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
  type OnConnect,
  type EdgeTypes,
  MarkerType,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Spinner, EmptyState } from "@/components/ui";
import type { FullSchema, SchemaInfo, Relationship } from "@/lib/types";

// Colors for table nodes by engine type.
const TABLE_COLORS = {
  postgres: { bg: "#eff6ff", border: "#3b82f6", header: "#dbeafe" },
  ferretdb: { bg: "#fef3c7", border: "#f59e0b", header: "#fde68a" },
  default:  { bg: "#f8fafc", border: "#94a3b8", header: "#f1f5f9" },
};

function tableNodeColor(engine: string) {
  return TABLE_COLORS[engine as keyof typeof TABLE_COLORS] || TABLE_COLORS.default;
}

function TableNode({ data }: { data: Record<string, any> }) {
  const schema = data.schema as SchemaInfo;
  const engine = (data.engine as string) || "default";
  const colors = tableNodeColor(engine);
  const pk = schema.columns.find((c) => c.is_primary);

  return (
    <div
      style={{
        background: colors.bg,
        border: `2px solid ${colors.border}`,
        borderRadius: 8,
        minWidth: 180,
        maxWidth: 260,
        fontSize: 12,
        fontFamily: "system-ui, sans-serif",
        boxShadow: "0 1px 4px rgba(0,0,0,0.1)",
      }}
    >
      <div
        style={{
          background: colors.header,
          padding: "6px 10px",
          borderRadius: "6px 6px 0 0",
          fontWeight: 600,
          borderBottom: `1px solid ${colors.border}`,
          color: "#1e293b",
        }}
      >
        {schema.collection}
      </div>
      <div style={{ padding: "4px 0" }}>
        {schema.columns.map((col) => (
          <div
            key={col.name}
            style={{
              padding: "2px 10px",
              display: "flex",
              alignItems: "center",
              gap: 6,
              color: "#475569",
            }}
          >
            <span style={{ fontSize: 10, color: "#94a3b8" }}>
              {col.is_primary ? "PK" : col.is_unique ? "UQ" : ""}
            </span>
            <span style={{ fontFamily: "monospace", fontSize: 11 }}>{col.name}</span>
            <span style={{ fontSize: 10, color: "#94a3b8", marginLeft: "auto" }}>
              {col.data_type}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

const nodeTypes: NodeTypes = {
  tableNode: TableNode as any,
};

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
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const supportsFK = schema?.capabilities.supports_foreign_keys ?? false;

  useEffect(() => {
    const token = authToken();
    if (!token) return;
    setLoading(true);
    api
      .getFullSchema(token, projectId)
      .then((s) => {
        setSchema(s);
        setError(null);
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Failed to load schema"))
      .finally(() => setLoading(false));
  }, [projectId]);

  useEffect(() => {
    if (!schema) return;

    const cols = schema.collections;
    const rels = schema.relationships;

    // Layout tables in a grid.
    const cols_per_row = Math.ceil(Math.sqrt(cols.length));
    const xGap = 300;
    const yGap = 200;

    const newNodes: Node[] = cols.map((c, i) => ({
      id: `table-${c.collection}`,
      type: "tableNode",
      position: {
        x: (i % cols_per_row) * xGap,
        y: Math.floor(i / cols_per_row) * yGap,
      },
      data: { schema: c, engine },
    }));

    const newEdges: Edge[] = rels.map((r, i) => ({
      id: `edge-${i}`,
      source: `table-${r.from_collection}`,
      target: `table-${r.to_collection}`,
      sourceHandle: r.from_column,
      targetHandle: r.to_column,
      type: "smoothstep",
      animated: true,
      style: { stroke: "#3b82f6", strokeWidth: 2 },
      markerEnd: { type: MarkerType.ArrowClosed, color: "#3b82f6" },
      label: `${r.from_column} → ${r.to_column}`,
      labelStyle: { fontSize: 10, fill: "#64748b" },
    }));

    setNodes(newNodes);
    setEdges(newEdges);
  }, [schema, engine, setNodes, setEdges]);

  if (loading) {
    return (
      <div className="flex items-center gap-2 py-8 text-sm text-slate-500">
        <Spinner className="h-4 w-4" /> Loading schema…
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700">
        {error}
      </div>
    );
  }

  if (!schema || schema.collections.length === 0) {
    return (
      <EmptyState
        title="No tables found"
        hint="Connect a database and ensure it has tables to visualize the schema."
      />
    );
  }

  const hasRelationships = schema.relationships.length > 0;

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-slate-800">
          Schema Explorer
          {!supportsFK && (
            <span className="ml-2 text-xs font-normal text-amber-600">
              (engine has no FK support — document/limited model, no relationship lines)
            </span>
          )}
        </h3>
        <span className="text-xs text-slate-500">
          {schema.collections.length} tables
          {hasRelationships && ` · ${schema.relationships.length} relationships`}
        </span>
      </div>

      {!hasRelationships && supportsFK && (
        <p className="text-xs text-slate-400">
          No foreign key relationships detected. Relationship lines will appear
          when tables have FK constraints.
        </p>
      )}

      <div style={{ height: 500, borderRadius: 8, border: "1px solid #e2e8f0" }}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          nodeTypes={nodeTypes}
          fitView
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
