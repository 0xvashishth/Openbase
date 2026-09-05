"use client";

import { useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Button, ErrorBanner, Input, Label } from "@/components/ui";

export function CreateProjectForm({ orgId, onCreated }: { orgId: string; onCreated?: () => void }) {
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
      const project = await api.createProject(token, orgId, name, slug);
      onCreated?.();
      window.location.href = `/orgs/${orgId}/projects/${project.id}`;
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create project");
      setLoading(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      {error && <ErrorBanner message={error} />}
      <div>
        <Label htmlFor="proj-name">Name</Label>
        <Input id="proj-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="My App" required />
      </div>
      <div>
        <Label htmlFor="proj-slug">Slug</Label>
        <Input id="proj-slug" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="my-app" required />
      </div>
      <p className="rounded-md border border-border bg-card px-3 py-2 text-label text-muted-foreground">
        After creation you&apos;ll pick a database engine (provisioned or
        bring-your-own) on the DB Source tab.
      </p>
      <Button type="submit" loading={loading}>
        Create project
      </Button>
    </form>
  );
}