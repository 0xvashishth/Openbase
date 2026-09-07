"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export default function ResetPasswordPage() {
  const router = useRouter();
  const params = useSearchParams();
  const token = params.get("token") ?? "";
  const [password, setPassword] = useState("");
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await api.resetPassword(token, password);
      setDone(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Reset failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-full items-center justify-center bg-background px-4 py-16">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-start">
          <h1 className="text-heading-sm font-w510 text-foreground-strong">
            Set a new password
          </h1>
          <p className="mt-2 text-body-sm text-muted-foreground">
            {done
              ? "Your password has been reset."
              : "Choose a strong password (8+ characters)."}
          </p>
        </div>

        {done ? (
          <Button
            variant="primary"
            className="w-full"
            onClick={() => router.push("/login")}
          >
            Sign in
          </Button>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            <div>
              <Label htmlFor="password">New password</Label>
              <Input
                id="password"
                type="password"
                required
                minLength={8}
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={loading || !token}
              />
            </div>

            {error && <p className="text-body-sm text-error">{error}</p>}

            {!token && (
              <p className="text-body-sm text-muted-foreground">
                Missing reset token. Open the link from your email.
              </p>
            )}

            <Button
              type="submit"
              variant="primary"
              disabled={loading || !token || password.length < 8}
              className="w-full"
            >
              {loading ? "Saving..." : "Save new password"}
            </Button>

            <Link
              href="/login"
              className="block text-center text-body-sm text-muted-foreground underline"
            >
              Back to sign in
            </Link>
          </form>
        )}
      </div>
    </div>
  );
}
