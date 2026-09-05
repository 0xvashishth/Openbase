"use client";

/**
 * Backwards-compatible re-exports.
 * New code should import from @/components/ui/* directly (shadcn).
 * This file keeps old `import { Button, Input, ... } from "@/components/ui"` working.
 */
import * as React from "react";
import { Button as NewButton, type ButtonProps as NewButtonProps } from "./ui/button";
import { Input as NewInput } from "./ui/input";
import { Label as NewLabel } from "./ui/label";
import { Card as NewCard } from "./ui/card";
import { Badge as NewBadge } from "./ui/badge";
import { EmptyState as NewEmpty, ErrorBanner as NewError, Spinner as NewSpinner } from "./ui/feedback";

export function Spinner({ className = "" }: { className?: string }) {
  return <NewSpinner className={className} />;
}

type LegacyVariant = "primary" | "secondary" | "danger" | "ghost" | "default" | "outline" | "destructive";

/**
 * Legacy variant names → design-system variants.
 *
 * `primary` now resolves to the real acid-lime CTA rather than being flattened
 * to `outline`. That was a placeholder from when the system had no accent; the
 * accent exists now, so the legacy name means what it says. Safe to promote:
 * no call site passes `variant="primary"` today, so nothing silently turns
 * lime — new opt-ins are explicit.
 *
 * `danger` → `destructive`, which is a coral OUTLINE in this system. Filled
 * coral is reserved for the confirm step inside ConfirmDialog.
 */
const variantMap: Record<LegacyVariant, NonNullable<NewButtonProps["variant"]>> = {
  primary: "primary",
  secondary: "secondary",
  danger: "destructive",
  ghost: "ghost",
  default: "outline",
  outline: "outline",
  destructive: "destructive",
};

export const Button = React.forwardRef<
  HTMLButtonElement,
  React.ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: LegacyVariant;
    /** Forwarded verbatim — the shim predates the size axis existing. */
    size?: NewButtonProps["size"];
    loading?: boolean;
    children: React.ReactNode;
  }
>(function Button({ variant = "outline", ...rest }, ref) {
  return <NewButton ref={ref} variant={variantMap[variant] ?? "outline"} {...rest} />;
});

export const Input = NewInput;
export function Label({ htmlFor, children }: { htmlFor?: string; children: React.ReactNode }) {
  return <NewLabel htmlFor={htmlFor}>{children}</NewLabel>;
}
export function Card({ children, className = "" }: { children: React.ReactNode; className?: string }) {
  return <NewCard className={className}>{children}</NewCard>;
}

/**
 * Legacy color-word tones → semantic Badge variants. `amber` maps to the
 * achromatic `warning`: DESIGN.md has no amber, and promoting a warning to the
 * accent would break the one-chromatic-element rule.
 */
const badgeToneMap = {
  green: "success",
  amber: "warning",
  red: "destructive",
  slate: "muted",
  blue: "secondary",
} as const;

export function Badge({
  tone = "slate",
  children,
}: {
  tone?: keyof typeof badgeToneMap;
  children: React.ReactNode;
}) {
  return <NewBadge variant={badgeToneMap[tone]}>{children}</NewBadge>;
}
export const EmptyState = NewEmpty;
export const ErrorBanner = NewError;
