"use client";

import { useParams, usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { ConnectionPanel } from "@/components/projects/ConnectionPanel";
import { TableBrowser } from "@/components/data/TableBrowser";
import { SchemaExplorer } from "@/components/schema/SchemaExplorer";
import { APIKeysPanel } from "@/components/projects/APIKeysPanel";
import { EmptyState, Spinner } from "@/components/ui";
import type { Project } from "@/lib/types";

const TABS = [
  { key: "overview", label: "Overview" },
  { key: "data", label: "Tables" },
  { key: "schema", label: "Schema" },
  { key: "api-keys", label: "API Keys" },
  { key: "connection", label: "Connection" },
] as const;

type TabKey = (typeof TABS)[number]["key"];

export default function ProjectDetailPage() {
  const params = useParams<{ orgId: string; projectId: string }>();
  const orgId = params.orgId;
  const projectId = params.projectId;
  const pathname = usePathname();
  const router = useRouter();

  const [project, setProject] = useState<Project | null>(null);
  const [hasConnection, setHasConnection] = useState<boolean | null>(null);
  const [engine, setEngine] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const activeTab: TabKey = pathname.endsWith("/connection")
    ? "connection"
    : pathname.endsWith("/schema")
    ? "schema"
    : pathname.endsWith("/api-keys")
    ? "api-keys"
    : pathname.endsWith("/data")
    ? "data"
    : "overview";

  useEffect(() => {
    const token = authToken();
    if (!token) return;
    api.listProjects(token, orgId).then((projects) => {
      const found = projects.find((p) => p.id === projectId);
      if (found) setProject(found);
      else setError("Project not found");
    });
    api
      .getConnection(token, projectId)
      .then((c) => {
        setHasConnection(c.status === "connected");
        setEngine(c.engine);
      })
      .catch(() => {
        setHasConnection(false);
        setEngine(null);
      });
  }, [orgId, projectId]);

  const go = (tab: TabKey) => {
    const base = `/orgs/${orgId}/projects/${projectId}`;
    router.push(tab === "overview" ? base : `${base}/${tab}`);
  };

  if (error) {
    return (
      <div className="px-6 py-8">
        <p className="text-sm text-red-600">{error}</p>
      </div>
    );
  }
  if (!project) {
    return (
      <div className="flex items-center gap-2 px-6 py-8 text-sm text-slate-500">
        <Spinner className="h-4 w-4" /> Loading project…
      </div>
    );
  }

  return (
    <div>
      <header className="border-b border-slate-200 bg-white px-6 py-5">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-semibold text-slate-900">{project.name}</h1>
          <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600 ring-1 ring-inset ring-slate-500/20">
            {project.slug}
          </span>
        </div>
      </header>

      <div className="border-b border-slate-200 bg-white px-6">
        <nav className="-mb-px flex gap-1">
          {TABS.map((tab) => (
            <button
              key={tab.key}
              onClick={() => go(tab.key)}
              className={`border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
                activeTab === tab.key
                  ? "border-brand-600 text-brand-700"
                  : "border-transparent text-slate-500 hover:border-slate-300 hover:text-slate-800"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      <div className="mx-auto max-w-6xl px-6 py-6">
        {activeTab === "overview" && (
          <div className="space-y-4">
            <div className="max-w-3xl rounded-xl border border-slate-200 bg-white p-5">
              <h2 className="text-sm font-semibold text-slate-800">Project ready</h2>
              <p className="mt-1 text-sm text-slate-500">
                This project is ready for a database. Connect one on the{" "}
                <button onClick={() => go("connection")} className="text-brand-600 hover:text-brand-700">
                  Connection
                </button>{" "}
                tab, then browse tables under{" "}
                <button onClick={() => go("data")} className="text-brand-600 hover:text-brand-700">
                  Tables
                </button>
                , or visualize the schema under{" "}
                <button onClick={() => go("schema")} className="text-brand-600 hover:text-brand-700">
                  Schema
                </button>
                .
              </p>
            </div>
          </div>
        )}

        {activeTab === "data" &&
          (hasConnection ? (
            <TableBrowser projectId={projectId} />
          ) : (
            <EmptyState
              title="Connect a database first"
              hint="Head to the Connection tab to attach a database, then come back to browse tables."
            />
          ))}

        {activeTab === "schema" &&
          (hasConnection ? (
            <SchemaExplorer projectId={projectId} engine={engine ?? "unknown"} />
          ) : (
            <EmptyState
              title="Connect a database first"
              hint="Head to the Connection tab to attach a database, then come back to visualize the schema."
            />
          ))}

        {activeTab === "api-keys" && <APIKeysPanel projectId={projectId} />}

        {activeTab === "connection" && <ConnectionPanel projectId={projectId} />}
      </div>
    </div>
  );
}
