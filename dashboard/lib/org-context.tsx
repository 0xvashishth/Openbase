"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { ApiError, api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { can } from "@/lib/permissions";
import type { Organization, OrgRole } from "@/lib/types";

export interface OrgContextValue {
  orgId: string;
  org: Organization | null;
  /** The caller's role in this org. Defaults to "member" until settled. */
  role: OrgRole;
  /** True once the org (and therefore the role) has loaded. */
  settled: boolean;
  /** Mirrors the Go RBAC matrix; see lib/permissions.ts. */
  can: (action: string) => boolean;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

const OrgContext = createContext<OrgContextValue | null>(null);

export function OrgProvider({ orgId, children }: { orgId: string; children: ReactNode }) {
  const [org, setOrg] = useState<Organization | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const refresh = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    let cancelled = false;
    const token = authToken();
    if (!token || !orgId) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    api
      .getOrg(token, orgId)
      .then((o) => {
        if (!cancelled) setOrg(o);
      })
      .catch((err) => {
        if (cancelled) return;
        const forbidden = err instanceof ApiError && err.status === 403;
        setError(
          forbidden
            ? "You do not have access to this organization"
            : err instanceof Error
              ? err.message
              : "Failed to load organization"
        );
        setOrg(null);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [orgId, nonce]);

  const value = useMemo<OrgContextValue>(() => {
    // Fail closed: until the role is known, assume the least privileged role so
    // destructive controls never flash enabled during load.
    const role: OrgRole = org?.role ?? "member";
    return {
      orgId,
      org,
      role,
      settled: !loading && org !== null,
      can: (action: string) => can(role, action),
      loading,
      error,
      refresh,
    };
  }, [orgId, org, loading, error, refresh]);

  return <OrgContext.Provider value={value}>{children}</OrgContext.Provider>;
}

export function useOrg(): OrgContextValue {
  const ctx = useContext(OrgContext);
  if (!ctx) throw new Error("useOrg must be used within OrgProvider");
  return ctx;
}

/**
 * Non-throwing variant for components that render both inside and outside an
 * org-scoped route (e.g. shared panels reused on the platform pages).
 */
export function useOrgOptional(): OrgContextValue | null {
  return useContext(OrgContext);
}
