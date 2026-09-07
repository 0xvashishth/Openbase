"use client";

import Link from "next/link";
import { useState } from "react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await api.forgotPassword(email);
      setSubmitted(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Request failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-full items-center justify-center bg-background px-4 py-16">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-start">
          <h1 className="text-heading-sm font-w510 text-foreground-strong">
            Reset your password
          </h1>
          <p className="mt-2 text-body-sm text-muted-foreground">
            {submitted
              ? "If that email is registered, a reset link is on its way."
              : "Enter your email and we'll send you a reset link."}
          </p>
        </div>

        {submitted ? (
          <Link href="/login" className="block text-center text-body-sm text-muted-foreground underline">
            Back to sign in
          </Link>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            <div>
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                required
                autoComplete="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                disabled={loading}
              />
            </div>

            {error && (
              <p className="text-body-sm text-error">{error}</p>
            )}

            <Button
              type="submit"
              variant="primary"
              disabled={loading || !email}
              className="w-full"
            >
              {loading ? "Sending..." : "Send reset link"}
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
