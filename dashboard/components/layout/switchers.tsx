"use client";

import * as React from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import type { Organization, Project } from "@/lib/types";

export function useOrgList(enabled = true): { orgs: Organization[] | null; loading: boolean } {
  const [orgs, setOrgs] = React.useState<Organization[] | null>(null);
  const [loading, setLoading] = React.useState(false);

  React.useEffect(() => {
    if (!enabled) return;
    const token = authToken();
    if (!token) return;
    let cancelled = false;
    setLoading(true);
    Promise.resolve(api.listOrgs(token))
      .then((list) => {
        if (!cancelled) setOrgs(Array.isArray(list) ? list : []);
      })
      .catch(() => {
        if (!cancelled) setOrgs([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled]);

  return { orgs, loading };
}

export function useProjectList(orgId?: string | null, enabled = true): {
  projects: Project[] | null;
  loading: boolean;
} {
  const [projects, setProjects] = React.useState<Project[] | null>(null);
  const [loading, setLoading] = React.useState(false);

  React.useEffect(() => {
    if (!enabled || !orgId) {
      setProjects(null);
      setLoading(false);
      return;
    }
    const token = authToken();
    if (!token) return;
    let cancelled = false;
    setLoading(true);
    Promise.resolve(api.listProjects(token, orgId))
      .then((list) => {
        if (!cancelled) setProjects(Array.isArray(list) ? list : []);
      })
      .catch(() => {
        if (!cancelled) setProjects([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [orgId, enabled]);

  return { projects, loading };
}
