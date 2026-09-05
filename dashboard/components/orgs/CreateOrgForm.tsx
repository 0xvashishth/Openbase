"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { api } from "@/lib/api";
import { Button, ErrorBanner, Input, Label } from "@/components/ui";
import { authToken } from "@/components/AuthProvider";

export function CreateOrgForm({ onCreated }: { onCreated?: () => void }) {
  const router = useRouter();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      const org = await api.createOrg(token, name, slug);
      onCreated?.();
      router.push(`/orgs/${org.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create org");
      setLoading(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      {error && <ErrorBanner message={error} />}
      <div>
        <Label htmlFor="org-name">Name</Label>
        <Input
          id="org-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Acme Inc."
          required
        />
      </div>
      <div>
        <Label htmlFor="org-slug">Slug</Label>
        <Input
          id="org-slug"
          value={slug}
          onChange={(e) => setSlug(e.target.value)}
          placeholder="acme"
          required
        />
        <p className="mt-1.5 text-label text-muted-foreground">
          Unique, URL-safe identifier used in the dashboard.
        </p>
      </div>
      <Button type="submit" loading={loading}>
        Create organization
      </Button>
    </form>
  );
}