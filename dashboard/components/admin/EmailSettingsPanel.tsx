"use client";

import { useEffect, useState } from "react";
import { api, ApiError } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Button, EmptyState, ErrorBanner, Input, Label } from "@/components/ui";
import { StatusBadge } from "@/components/ui/badge";
import { SegmentedOption } from "@/components/ui/segmented";
import { ToolPageSkeleton } from "@/components/ui/skeletons";
import { useToast } from "@/components/ui/toast";
import type { MailLogEntry, MailProvider, MailSettingsView } from "@/lib/types";

/**
 * Platform Email settings (Phase 9.2): the operator's own mail provider
 * (BYOC SMTP) for verification, reset and invite emails. Owner-gated
 * server-side; non-owners get an explanation instead of a broken form.
 */
export function EmailSettingsPanel() {
  const toast = useToast();
  const [settings, setSettings] = useState<MailSettingsView | null>(null);
  const [log, setLog] = useState<MailLogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [forbidden, setForbidden] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [provider, setProvider] = useState<MailProvider>("log");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("587");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [fromAddress, setFromAddress] = useState("");
  const [fromName, setFromName] = useState("Openbase");
  const [saving, setSaving] = useState(false);

  const [testTo, setTestTo] = useState("");
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; provider: string; error?: string } | null>(null);

  async function load() {
    const token = authToken();
    if (!token) return;
    try {
      const [s, entries] = await Promise.all([
        api.getMailSettings(token),
        api.listMailLog(token, 50).catch(() => [] as MailLogEntry[]),
      ]);
      setSettings(s);
      setLog(entries);
      setProvider(s.provider);
      setHost(s.smtp_host);
      setPort(String(s.smtp_port || 587));
      setUsername(s.smtp_username);
      setPassword("");
      setFromAddress(s.from_address);
      setFromName(s.from_name || "Openbase");
    } catch (err) {
      if (err instanceof ApiError && err.status === 403) {
        setForbidden(true);
      } else {
        setError(err instanceof Error ? err.message : "Failed to load email settings");
      }
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      const portNum = parseInt(port, 10);
      const updated = await api.updateMailSettings(token, {
        provider,
        smtp_host: host.trim(),
        smtp_port: Number.isFinite(portNum) ? portNum : 587,
        smtp_username: username.trim(),
        // Empty password keeps the stored secret (server-side semantics).
        ...(password ? { smtp_password: password } : {}),
        from_address: fromAddress.trim(),
        from_name: fromName.trim() || "Openbase",
      });
      setSettings(updated);
      setPassword("");
      toast.success("Email settings saved", provider === "smtp" ? `SMTP via ${updated.smtp_host}.` : "Log-only mode.");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save email settings");
    } finally {
      setSaving(false);
    }
  }

  async function sendTest(e: React.FormEvent) {
    e.preventDefault();
    if (!testTo.trim()) return;
    setTesting(true);
    setTestResult(null);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      const res = await api.testMailSend(token, testTo.trim());
      setTestResult(res);
      if (res.ok) {
        toast.success("Test email sent", `Delivered via ${res.provider} to ${testTo.trim()}.`);
      }
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to send test email");
    } finally {
      setTesting(false);
    }
  }

  if (loading) {
    return <ToolPageSkeleton label="Loading email settings" />;
  }

  if (forbidden) {
    return (
      <EmptyState
        title="Owner access required"
        hint="Platform email settings are managed by an organization owner. Ask an owner to configure the mail provider."
      />
    );
  }

  return (
    <div className="space-y-4">
      <section className="max-w-2xl">
        <h2 className="mb-2 text-caption font-w510 text-foreground-strong">Provider</h2>
        <div className="flex gap-1">
          <SegmentedOption selected={provider === "smtp"} onClick={() => setProvider("smtp")} className="h-8 flex-1">
            SMTP (your own)
          </SegmentedOption>
          <SegmentedOption selected={provider === "log"} onClick={() => setProvider("log")} className="h-8 flex-1">
            Log only
          </SegmentedOption>
        </div>
        <p className="mt-2 text-label text-muted-foreground">
          {provider === "smtp"
            ? "Bring your own credentials — Gmail, Mailgun, SES via SMTP relay, or any relay. The password is encrypted at rest and never shown again."
            : "No provider configured. Verification, reset and invite emails are logged server-side instead of sent — fine for local development."}
        </p>
      </section>

      <section className="max-w-2xl">
        <h2 className="mb-2 text-caption font-w510 text-foreground-strong">SMTP settings</h2>
        <form onSubmit={save} className="space-y-3">
          <div className="grid gap-3 sm:grid-cols-[1fr_120px]">
            <div>
              <Label htmlFor="mail-host">Host</Label>
              <Input
                id="mail-host"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                placeholder="smtp.example.com"
                disabled={provider !== "smtp"}
              />
            </div>
            <div>
              <Label htmlFor="mail-port">Port</Label>
              <Input
                id="mail-port"
                value={port}
                onChange={(e) => setPort(e.target.value)}
                placeholder="587"
                disabled={provider !== "smtp"}
              />
            </div>
          </div>
          <div className="text-label text-muted-foreground">
            Port 465 uses implicit TLS; any other port requires STARTTLS — plaintext delivery is refused.
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <Label htmlFor="mail-user">Username</Label>
              <Input
                id="mail-user"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="ops@example.com"
                disabled={provider !== "smtp"}
                autoComplete="off"
              />
            </div>
            <div>
              <Label htmlFor="mail-pass">
                Password {settings?.password_set && !password ? "(stored — leave blank to keep)" : ""}
              </Label>
              <Input
                id="mail-pass"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={settings?.password_set ? "••••••••" : "SMTP password / app password"}
                disabled={provider !== "smtp"}
                autoComplete="new-password"
              />
            </div>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <Label htmlFor="mail-from">From address</Label>
              <Input
                id="mail-from"
                value={fromAddress}
                onChange={(e) => setFromAddress(e.target.value)}
                placeholder="noreply@example.com"
              />
            </div>
            <div>
              <Label htmlFor="mail-from-name">From name</Label>
              <Input
                id="mail-from-name"
                value={fromName}
                onChange={(e) => setFromName(e.target.value)}
                placeholder="Openbase"
              />
            </div>
          </div>
          <Button type="submit" loading={saving}>
            Save email settings
          </Button>
        </form>
      </section>

      {error && <ErrorBanner message={error} />}

      <section className="max-w-2xl">
        <h2 className="mb-2 text-caption font-w510 text-foreground-strong">Send a test email</h2>
        <form onSubmit={sendTest} className="flex items-end gap-3">
          <div className="flex-1">
            <Label htmlFor="mail-test-to">Recipient</Label>
            <Input
              id="mail-test-to"
              type="email"
              value={testTo}
              onChange={(e) => setTestTo(e.target.value)}
              placeholder="you@example.com"
            />
          </div>
          <Button type="submit" loading={testing} disabled={!testTo.trim()}>
            Send test
          </Button>
        </form>
        {testResult && (
          <div
            className={
              testResult.ok
                ? "mt-2 rounded-md border border-success/30 bg-success/10 px-3 py-2 text-caption text-success"
                : "mt-2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-caption text-destructive"
            }
          >
            {testResult.ok
              ? `Delivered via ${testResult.provider}. Check the inbox (and spam).`
              : `Failed via ${testResult.provider}: ${testResult.error ?? "unknown error"}`}
          </div>
        )}
      </section>

      <section className="max-w-2xl">
        <div className="mb-2 flex items-center gap-3">
          <h2 className="text-caption font-w510 text-foreground-strong">Delivery log</h2>
          <Button variant="ghost" onClick={() => void load()}>
            Refresh
          </Button>
        </div>
        {log.length === 0 ? (
          <EmptyState
            title="No sends yet"
            hint="Save settings and send a test email — every attempt lands here with its outcome."
          />
        ) : (
          <div className="space-y-2">
            {log.map((e) => (
              <div key={e.id} className="rounded-lg border border-border bg-card px-4 py-3">
                <div className="flex flex-wrap items-center gap-2">
                  {e.ok ? (
                    <StatusBadge tone="success">sent</StatusBadge>
                  ) : (
                    <StatusBadge tone="destructive">failed</StatusBadge>
                  )}
                  <span className="truncate font-mono text-caption text-foreground-strong">{e.to_address}</span>
                  <span className="text-label text-muted-foreground">{e.template}</span>
                </div>
                <div className="mt-1 text-label text-muted-foreground">
                  {e.subject} · {new Date(e.created_at).toLocaleString()}
                </div>
                {!e.ok && e.error && (
                  <div className="mt-1 font-mono text-label text-destructive">{e.error}</div>
                )}
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
