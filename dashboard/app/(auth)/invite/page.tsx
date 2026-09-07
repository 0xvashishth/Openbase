"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

function AcceptInviteForm() {
  const router = useRouter();
  const params = useSearchParams();
  const token = params.get("token") ?? "";
  const [fullName, setFullName] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (password.length < 8) {
      setError("Password must be at least 8 characters");
      return;
    }
    setLoading(true);
    try {
      const res = await api.acceptInvite(token, password, fullName);
      router.push(`/orgs/${res.org_id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Accept failed");
      setLoading(false);
    }
  }

  return (
    <div className="w-full max-w-sm">
      <div className="mb-8 flex flex-col items-start">
        <h1 className="text-heading-sm font-w510 text-foreground-strong">
          Accept your invitation
        </h1>
        <p className="mt-2 text-body-sm text-muted-foreground">
          Set up your account to join the organization.
        </p>
      </div>

      <form onSubmit={submit} className="space-y-4">
        <div>
          <Label htmlFor="fullName">Full name</Label>
          <Input
            id="fullName"
            value={fullName}
            onChange={(e) => setFullName(e.target.value)}
            required
            disabled={loading}
          />
        </div>
        <div>
          <Label htmlFor="password">Password</Label>
          <Input
            id="password"
            type="password"
            minLength={8}
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            disabled={loading}
          />
        </div>

        {error && <p className="text-body-sm text-error">{error}</p>}
        {!token && (
          <p className="text-body-sm text-muted-foreground">
            Missing invite token. Open the link from your email.
          </p>
        )}

        <Button
          type="submit"
          variant="primary"
          disabled={loading || !token}
          className="w-full"
        >
          {loading ? "Joining..." : "Accept and join"}
        </Button>

        <Link
          href="/login"
          className="block text-center text-body-sm text-muted-foreground underline"
        >
          Already have an account? Sign in
        </Link>
      </form>
    </div>
  );
}

export default function AcceptInvitePage() {
  return (
    <div className="flex min-h-full items-center justify-center bg-background px-4 py-16">
      <Suspense fallback={null}>
        <AcceptInviteForm />
      </Suspense>
    </div>
  );
}
