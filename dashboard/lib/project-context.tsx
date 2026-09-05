"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { ApiError, api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import type { CapabilitySet, Connection, Project } from "@/lib/types";

export interface ProjectContextValue {
  orgId: string;
  projectId: string;
  project: Project | null;
  connection: Connection | null;
  hasConnection: boolean;
  engine: string | null;
  capabilities: CapabilitySet | null;
  supportsTriggers: boolean;
  supportsRealtime: boolean;
  supportsForeignKeys: boolean;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

const ProjectContext = createContext<ProjectContextValue | null>(null);

const DEFAULT_CAPS: CapabilitySet = {
  supports_relational_joins: false,
  supports_foreign_keys: false,
  supports_native_triggers: false,
  supports_change_streams: false,
  supports_realtime: "none",
  supports_transactions: false,
  supports_full_text_search: false,
  supports_vector_search: false,
};

export function ProjectProvider({
  orgId,
  projectId,
  children,
}: {
  orgId: string;
  projectId: string;
  children: ReactNode;
}) {
  const [project, setProject] = useState<Project | null>(null);
  const [connection, setConnection] = useState<Connection | null>(null);
  const [capabilities, setCapabilities] = useState<CapabilitySet | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const refresh = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    let cancelled = false;
    const token = authToken();
    if (!token) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    (async () => {
      try {
        // Resolve the one project directly. This used to list every project in
        // the org and scan for an id we already had.
        const found = await api.getProject(token, projectId);
        if (cancelled) return;
        setProject(found);
        try {
          const conn = await api.getConnection(token, projectId);
          if (cancelled) return;
          setConnection(conn);
          if (conn.status === "connected") {
            try {
              const schema = await api.getFullSchema(token, projectId);
              if (!cancelled) setCapabilities(schema.capabilities);
            } catch {
              if (!cancelled) setCapabilities(DEFAULT_CAPS);
            }
          } else {
            setCapabilities(DEFAULT_CAPS);
          }
        } catch {
          if (!cancelled) {
            setConnection(null);
            setCapabilities(DEFAULT_CAPS);
          }
        }
      } catch (err) {
        if (cancelled) return;
        // A 404 is a missing project, not an infrastructure failure; keep the
        // existing "Project not found" copy the guard renders.
        const notFound = err instanceof ApiError && err.status === 404;
        setError(notFound ? "Project not found" : err instanceof Error ? err.message : "Failed to load project");
        setProject(null);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [orgId, projectId, nonce]);

  const value = useMemo<ProjectContextValue>(() => {
    const hasConnection = connection?.status === "connected";
    const caps = capabilities ?? DEFAULT_CAPS;
    return {
      orgId,
      projectId,
      project,
      connection,
      hasConnection,
      engine: connection?.engine ?? null,
      capabilities: capabilities,
      supportsTriggers: caps.supports_native_triggers,
      supportsRealtime: caps.supports_realtime === "native",
      supportsForeignKeys: caps.supports_foreign_keys,
      loading,
      error,
      refresh,
    };
  }, [orgId, projectId, project, connection, capabilities, loading, error, refresh]);

  return <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>;
}

export function useProject(): ProjectContextValue {
  const ctx = useContext(ProjectContext);
  if (!ctx) throw new Error("useProject must be used within ProjectProvider");
  return ctx;
}
