"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useAuth } from "@/components/AuthProvider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ErrorBanner } from "@/components/ui/feedback";

export default function LoginPage() {
  const router = useRouter();
  const { login } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await login(email, password);
      router.push("/orgs");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
      setLoading(false);
    }
  }

  return (
    // Auth is one of only two surfaces in the app where DESIGN.md's display
    // scale applies — a single focal point on a full-bleed canvas, rather than
    // the compact data density of the dashboard proper.
    <div className="flex min-h-full items-center justify-center bg-background px-4 py-16">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-start">
          {/* § Logo Mark: achromatic. The acid-lime accent belongs to the
              submit button below — the single primary action on this view. */}
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-foreground-strong text-caption font-w510 text-background">
            O
          </div>
          <h1 className="mt-5 text-heading-sm font-w510 text-foreground-strong">Sign in to Openbase</h1>
          <p className="mt-2 text-body-sm text-muted-foreground">
            Self-hosted backend for any database
          </p>
        </div>

        <form onSubmit={submit} className="space-y-4 rounded-lg border border-border bg-card p-6">
          {error && <ErrorBanner message={error} />}
          <div>
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              type="email"
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
            />
          </div>
          <div>
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          {/* The one acid-lime element on the page. */}
          <Button type="submit" variant="primary" size="lg" loading={loading} className="w-full">
            Sign in
          </Button>
        </form>

        <p className="mt-6 text-caption text-muted-foreground">
          No account?{" "}
          <Link
            href="/register"
            className="text-foreground underline decoration-border decoration-1 underline-offset-4 transition-colors hover:decoration-foreground"
          >
            Create one
          </Link>
        </p>

        <p className="mt-2 text-caption text-muted-foreground">
          <Link
            href="/forgot-password"
            className="text-foreground underline decoration-border decoration-1 underline-offset-4 transition-colors hover:decoration-foreground"
          >
            Forgot password?
          </Link>
        </p>
      </div>
    </div>
  );
}
