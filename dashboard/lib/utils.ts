import { clsx, type ClassValue } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";

/**
 * Custom scale values from tailwind.config.ts.
 *
 * tailwind-merge ships with knowledge of Tailwind's DEFAULT scales only. Any
 * bespoke value has to be declared here or the merge misclassifies it — most
 * damagingly, an unknown `text-*` is assumed to be a COLOR, so
 * `cn("text-caption", "text-foreground")` silently drops the color and
 * `cn("text-foreground", "text-caption")` silently drops the size. Both are
 * common in this codebase (variant base classes + per-call-site overrides).
 */
const FONT_SIZES = [
  "micro",
  "label",
  "caption",
  "body-sm",
  "body",
  "body-lg",
  "body-emphasis",
  "heading",
  "heading-sm",
  "heading-lg",
  "hero",
  "display",
] as const;

const twMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      "font-size": [{ text: [...FONT_SIZES] }],
      // Non-standard weight stops. Declared under `font` so they conflict with
      // font-normal/medium/semibold but NOT with font-sans/font-mono.
      "font-weight": [{ font: ["w510", "w590"] }],
      rounded: [{ rounded: ["badge"] }],
      "max-w": [{ "max-w": ["shell"] }],
      "border-w": [{ border: ["hairline"] }],
    },
  },
});

/**
 * Merge class names, shadcn-style.
 * Handles conditional classes + Tailwind conflicts (e.g. px-2 vs px-4).
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
