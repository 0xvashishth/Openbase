import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * Design-system invariants (DESIGN.md).
 *
 * These are source-level guards, not render tests. Several rules in DESIGN.md
 * are absolute ("never use bold weights", "three radii is the entire radius
 * vocabulary"), and a component test can only ever catch the one component it
 * renders. Scanning source catches the next hand-rolled `<select>` or
 * copy-pasted `text-[11px]` before it ships.
 *
 * Two rules are additionally enforced by tailwind.config.ts, which remaps the
 * offending utilities so they cannot resolve to out-of-band values even if
 * someone writes them. These tests keep the intent visible and catch the case
 * where the config remap is removed.
 */

const ROOT = join(__dirname, "..");
const SCAN_DIRS = ["app", "components", "lib"];

function collect(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    if (entry === "node_modules" || entry === ".next") continue;
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      out.push(...collect(full));
      continue;
    }
    if (!/\.tsx?$/.test(entry)) continue;
    if (/\.test\.tsx?$/.test(entry)) continue;
    out.push(full);
  }
  return out;
}

const FILES = SCAN_DIRS.flatMap((d) => collect(join(ROOT, d))).map((path) => ({
  path: path.slice(ROOT.length + 1),
  source: readFileSync(path, "utf8"),
}));

/** Reports every offending file so a failure names all sites at once. */
function offenders(pattern: RegExp, exempt: (path: string) => boolean = () => false) {
  return FILES.filter((f) => !exempt(f.path))
    .map((f) => ({ path: f.path, hits: f.source.match(pattern) ?? [] }))
    .filter((f) => f.hits.length > 0)
    .map((f) => `${f.path}: ${[...new Set(f.hits)].join(", ")}`);
}

describe("design system invariants", () => {
  it("scans a meaningful number of files", () => {
    // Guards the guard: a broken glob would make every test below vacuous.
    expect(FILES.length).toBeGreaterThan(50);
  });

  /** DESIGN.md § Don't: "Do not use bold weights (700+)". */
  it("uses no font weight above 590", () => {
    expect(offenders(/font-(?:bold|extrabold|black)\b/g)).toEqual([]);
  });

  /**
   * § Do's: "Set card radius to 12px, button radius to 6px, pill radius to
   * 9999px — three radii is the entire radius vocabulary" (+ 2px/4px chips).
   * `rounded-xl` and above are remapped to 12px in the config, but writing them
   * implies an intent the system does not have.
   */
  it("uses no radius utility above the 12px card maximum", () => {
    expect(offenders(/rounded-(?:2xl|3xl)\b/g)).toEqual([]);
  });

  /**
   * § Don't: "Do not introduce additional chromatic accent colors". Tailwind's
   * stock palette is not part of this system — every color must resolve through
   * a token.
   */
  it("uses no raw Tailwind palette colors", () => {
    const palette =
      /\b(?:bg|text|border|ring|fill|stroke|decoration|from|to|via)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}\b/g;
    expect(offenders(palette)).toEqual([]);
  });

  /**
   * The type scale is a closed set of named roles. Arbitrary pixel sizes bypass
   * the size/line-height/tracking triple and drift out of the scale.
   */
  it("uses no arbitrary font sizes", () => {
    expect(offenders(/text-\[\d+(?:\.\d+)?px\]/g)).toEqual([]);
  });

  /**
   * § Don't: "Do not use shadows to separate cards from the canvas". The only
   * sanctioned shadows are the primary button's inset stack (`shadow-subtle-3`)
   * and overlay elevation (`shadow-xl`).
   */
  it("uses no ad-hoc drop shadows for surface separation", () => {
    const banned = /\b(?:hover:)?shadow-(?:sm|md|lg|inner|2xl)\b|drop-shadow/g;
    expect(offenders(banned, (p) => p === "tailwind.config.ts")).toEqual([]);
  });

  /**
   * Focus rings settled on 1px system-wide; 2px is shadcn's default and reads
   * as a halo at these radii.
   */
  it("uses 1px focus rings", () => {
    expect(offenders(/ring-2\b/g)).toEqual([]);
  });

  /**
   * The acid-lime accent is the single primary action per view, so it must never
   * be reachable through a Badge variant or a bare fill on a nav item.
   * `bg-primary/10` (a 10% wash behind an active indicator) is allowed.
   */
  it("never fills a badge with the accent", () => {
    const badge = FILES.find((f) => f.path === "components/ui/badge.tsx");
    expect(badge).toBeDefined();
    expect(badge!.source).not.toMatch(/bg-primary(?!\/)/);
    expect(badge!.source).not.toMatch(/acid-lime/);
  });
});
