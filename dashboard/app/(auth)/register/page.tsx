"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useAuth } from "@/components/AuthProvider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ErrorBanner } from "@/components/ui/feedback";

export default function RegisterPage() {
  const router = useRouter();
  const { register } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [fullName, setFullName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await register(email, password, fullName || undefined);
      router.push("/orgs");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Registration failed");
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-full items-center justify-center bg-background px-4 py-16">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-start">
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-foreground-strong text-caption font-w510 text-background">
            O
          </div>
          <h1 className="mt-5 text-heading-sm font-w510 text-foreground-strong">Create your account</h1>
          <p className="mt-2 text-body-sm text-muted-foreground">
            Start with an organization and a project
          </p>
        </div>

        <form onSubmit={submit} className="space-y-4 rounded-lg border border-border bg-card p-6">
          {error && <ErrorBanner message={error} />}
          <div>
            <Label htmlFor="fullName">Full name (optional)</Label>
            <Input
              id="fullName"
              value={fullName}
              onChange={(e) => setFullName(e.target.value)}
              autoComplete="name"
            />
          </div>
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
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
            <p className="mt-1.5 text-label text-muted-foreground">Use at least 8 characters.</p>
          </div>
          {/* The one acid-lime element on the page. */}
          <Button type="submit" variant="primary" size="lg" loading={loading} className="w-full">
            Create account
          </Button>
        </form>

        <p className="mt-6 text-caption text-muted-foreground">
          Already have an account?{" "}
          <Link
            href="/login"
            className="text-foreground underline decoration-border decoration-1 underline-offset-4 transition-colors hover:decoration-foreground"
          >
            Sign in
          </Link>
        </p>
      </div>
    </div>
  );
}
