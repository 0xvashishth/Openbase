"use client";

/**
 * Backwards-compatible re-exports.
 * New code should import from @/components/ui/* directly (shadcn).
 * This file keeps old `import { Button, Input, ... } from "@/components/ui"` working.
 */
import * as React from "react";
import { Button as NewButton } from "./ui/button";
import { Input as NewInput } from "./ui/input";
import { Label as NewLabel } from "./ui/label";
import { Card as NewCard } from "./ui/card";
import { LegacyBadge } from "./ui/badge";
import { EmptyState as NewEmpty, ErrorBanner as NewError, Spinner as NewSpinner } from "./ui/feedback";

export function Spinner({ className = "" }: { className?: string }) {
  return <NewSpinner className={className} />;
}

type LegacyVariant = "primary" | "secondary" | "danger" | "ghost" | "default" | "outline" | "destructive";

const variantMap: Record<LegacyVariant, "default" | "secondary" | "destructive" | "ghost" | "outline"> = {
  // Outline-by-default design: primary/secondary/default all render as outline.
  // Only explicit destructive/danger stay solid; ghost stays minimal.
  primary: "outline",
  secondary: "outline",
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
export function Badge({
  tone = "slate",
  children,
}: {
  tone?: "green" | "amber" | "red" | "slate" | "blue";
  children: React.ReactNode;
}) {
  return <LegacyBadge tone={tone}>{children}</LegacyBadge>;
}
export const EmptyState = NewEmpty;
export const ErrorBanner = NewError;
