"use client";

import * as React from "react";
import useSWR from "swr";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import type { Organization, Project } from "@/lib/types";

/**
 * Shared, cached data hooks (Phase 8.10). Previously every consumer fired its
 * own hand-rolled fetch — a single project page triggered listOrgs 3×
 * concurrently plus a 30s repoll. SWR deduplicates in-flight requests and
 * shares cache across StatusBar, Topbar and GlobalSearch.
 *
 * Return shapes are unchanged ({ orgs, loading } / { projects, loading }) so
 * existing callers keep working.
 */

// Revalidate cached lists at most this often while mounted.
const DEDUPING_MS = 10_000;

function sessionKey(): string | null {
  // Keyed by token presence so login/logout swaps cache scope. The raw token
  // stays out of the key — SWR keys surface in devtools.
  const token = authToken();
  if (!token) return null;
  let h = 0;
  for (let i = 0; i < token.length; i++) h = (h * 31 + token.charCodeAt(i)) | 0;
  return `sess-${(h >>> 0).toString(36)}`;
}

export function useOrgList(enabled = true): { orgs: Organization[] | null; loading: boolean } {
  const sess = sessionKey();
  const { data, isLoading } = useSWR(
    enabled && sess ? ["orgs", sess] : null,
    async () => {
      const token = authToken();
      if (!token) return [] as Organization[];
      const list = await api.listOrgs(token);
      return Array.isArray(list) ? list : [];
    },
    { dedupingInterval: DEDUPING_MS }
  );
  return { orgs: data ?? null, loading: isLoading };
}

export function useProjectList(orgId?: string | null, enabled = true): {
  projects: Project[] | null;
  loading: boolean;
} {
  const sess = sessionKey();
  const { data, isLoading } = useSWR(
    enabled && orgId && sess ? ["projects", orgId, sess] : null,
    async () => {
      const token = authToken();
      if (!token || !orgId) return [] as Project[];
      const list = await api.listProjects(token, orgId);
      return Array.isArray(list) ? list : [];
    },
    { dedupingInterval: DEDUPING_MS }
  );
  return { projects: data ?? null, loading: isLoading };
}

/** Imperatively refresh the shared lists after mutations (create org/project). */
export async function refreshOrgLists(): Promise<void> {
  const { mutate } = await import("swr");
  await mutate((key) => Array.isArray(key) && (key[0] === "orgs" || key[0] === "projects"));
}
