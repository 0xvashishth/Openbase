"use client";

import { useEffect, useState } from "react";
import { api, API_URL } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Button, EmptyState, ErrorBanner, Input, Label } from "@/components/ui";
import { Badge } from "@/components/ui/badge";
import { CopyField } from "@/components/ui/code-block";
import { ConfirmDialog } from "@/components/ui/dialog";
import { ListSkeleton } from "@/components/ui/skeletons";
import { useToast } from "@/components/ui/toast";
import type { AuthProviderView } from "@/lib/types";

const KNOWN = [
  { id: "github", label: "GitHub", hint: "OAuth App client ID + secret." },
  { id: "google", label: "Google", hint: "OAuth client ID + secret (Cloud Console)." },
  { id: "oidc", label: "Custom OIDC", hint: "Any OpenID Connect issuer via discovery." },
  { id: "sms", label: "SMS (phone OTP)", hint: "Twilio credentials, or log-only fallback." },
] as const;

function callbackUrl(provider: string): string {
  const base = API_URL || window.location.origin;
  return `${base.replace(/\/+$/, "")}/auth/v1/callback?provider=${provider}`;
}

export function AuthProvidersPanel({ projectId }: { projectId: string }) {
  const toast = useToast();
  const [rows, setRows] = useState<AuthProviderView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [configText, setConfigText] = useState("{}");
  const [saving, setSaving] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<string | null>(null);

  async function load() {
    const token = authToken();
    if (!token) return;
    setLoading(true);
    setError(null);
    try {
      setRows(await api.listAuthProviders(token, projectId));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load providers");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  const byId = new Map(rows.map((r) => [r.provider, r]));

  function startEdit(id: string) {
    const existing = byId.get(id);
    setEditing(id);
    setClientId(existing?.client_id ?? "");
    setClientSecret("");
    setConfigText(JSON.stringify(existing?.config ?? defaultConfig(id), null, 2));
    setError(null);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!editing) return;
    let config: Record<string, unknown>;
    try {
      config = JSON.parse(configText || "{}");
    } catch {
      setError("Config must be valid JSON");
      return;
    }
    const token = authToken();
    if (!token) return;
    setSaving(true);
    setError(null);
    try {
      await api.upsertAuthProvider(token, projectId, {
        provider: editing,
        enabled: true,
        client_id: clientId.trim() || undefined,
        client_secret: clientSecret.trim() || undefined,
        config,
      });
      toast.success(`${editing} provider saved`, clientSecret.trim() ? "Secret stored encrypted." : undefined);
      setEditing(null);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save provider");
    } finally {
      setSaving(false);
    }
  }

  async function toggle(provider: string, enabled: boolean) {
    const token = authToken();
    if (!token) return;
    try {
      await api.upsertAuthProvider(token, projectId, { provider, enabled });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to toggle provider");
    }
  }

  async function remove() {
    if (!pendingDelete) return;
    const token = authToken();
    if (!token) return;
    try {
      await api.deleteAuthProvider(token, projectId, pendingDelete);
      setPendingDelete(null);
      toast.success("Provider removed");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to remove provider");
    }
  }

  if (loading) return <ListSkeleton />;

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} />}
      <ul className="divide-y divide-border rounded-md border">
        {KNOWN.map(({ id, label, hint }) => {
          const row = byId.get(id);
          const open = editing === id;
          return (
            <li key={id} className="space-y-3 px-3 py-3">
              <div className="flex flex-wrap items-center gap-3">
                <div className="min-w-0 flex-1">
                  <p className="text-body-sm font-medium">{label}</p>
                  <p className="text-caption text-muted-foreground">{hint}</p>
                </div>
                {row ? <Badge>{row.enabled ? "enabled" : "disabled"}</Badge> : <Badge>not configured</Badge>}
                {row?.has_secret && <Badge>secret stored</Badge>}
                <Button size="sm" variant="outline" onClick={() => (open ? setEditing(null) : startEdit(id))}>
                  {open ? "Close" : row ? "Configure" : "Set up"}
                </Button>
                {row && (
                  <Button size="sm" variant="outline" onClick={() => toggle(id, !row.enabled)}>
                    {row.enabled ? "Disable" : "Enable"}
                  </Button>
                )}
                {row && (
                  <Button size="sm" variant="destructive" onClick={() => setPendingDelete(id)}>
                    Remove
                  </Button>
                )}
              </div>
              {id !== "sms" && (
                <CopyField label="Callback URL (paste into the provider app)" value={callbackUrl(id)} />
              )}
              {open && (
                <form className="grid gap-3 rounded-md border p-3" onSubmit={save}>
                  {id !== "sms" && (
                    <div className="grid gap-1">
                      <Label htmlFor={`provider-cid-${id}`}>Client ID</Label>
                      <Input
                        id={`provider-cid-${id}`}
                        value={clientId}
                        onChange={(e) => setClientId(e.target.value)}
                        placeholder={id === "oidc" ? "not required for most issuers" : "oauth client id"}
                      />
                    </div>
                  )}
                  <div className="grid gap-1">
                    <Label htmlFor={`provider-secret-${id}`}>
                      {id === "sms" ? "Auth token (Twilio)" : "Client secret (write-only, encrypted at rest)"}
                    </Label>
                    <Input
                      id={`provider-secret-${id}`}
                      type="password"
                      value={clientSecret}
                      onChange={(e) => setClientSecret(e.target.value)}
                      placeholder={row?.has_secret ? "•••••• (leave blank to keep)" : "paste secret"}
                    />
                  </div>
                  <div className="grid gap-1">
                    <Label htmlFor={`provider-config-${id}`}>Config (JSON)</Label>
                    <Input
                      id={`provider-config-${id}`}
                      value={configText}
                      onChange={(e) => setConfigText(e.target.value)}
                    />
                    <p className="text-caption text-muted-foreground">{configHint(id)}</p>
                  </div>
                  <div>
                    <Button type="submit" disabled={saving}>
                      {saving ? "Saving…" : "Save provider"}
                    </Button>
                  </div>
                </form>
              )}
            </li>
          );
        })}
      </ul>
      {rows.length === 0 && (
        <EmptyState
          title="Password auth is always on"
          hint="Email + password works with no configuration. Add an OAuth or SMS driver above when your app needs it."
        />
      )}
      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => !open && setPendingDelete(null)}
        title="Remove this provider?"
        description="Users who signed in with it keep their accounts, but new logins through it stop working."
        confirmLabel="Remove"
        onConfirm={remove}
      />
    </div>
  );
}

function defaultConfig(id: string): Record<string, unknown> {
  switch (id) {
    case "oidc":
      return { issuer: "https://accounts.example.com", scopes: "openid email profile" };
    case "sms":
      return { driver: "log", from: "+1555000000" };
    default:
      return {};
  }
}

function configHint(id: string): string {
  switch (id) {
    case "oidc":
      return "issuer (required), scopes, optional endpoint overrides: auth_url, token_url, userinfo_url.";
    case "sms":
      return 'driver: "log" records instead of sending; "twilio" needs account_sid + from (secret = auth token). Optional base_url override for tests.';
    default:
      return "Optional endpoint overrides for tests: auth_url, token_url, userinfo_url (github: emails_url).";
  }
}
