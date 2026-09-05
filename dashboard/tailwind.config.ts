import type { Config } from "tailwindcss";

/**
 * Openbase design system — Tailwind v3 binding for DESIGN.md.
 *
 * DESIGN.md ships its Quick Start as Tailwind v4 `@theme`. This project is on
 * v3.4, so tokens live in `app/globals.css` as custom properties and are bound
 * to utilities here. Colors resolve through `rgb(var(--token) / <alpha-value>)`
 * so the `/opacity` modifier keeps working (`bg-card/40`, `border-foreground/25`).
 *
 * Three of the extensions below intentionally REMAP Tailwind defaults rather
 * than adding new names. That makes existing call sites comply with the spec
 * without touching them:
 *
 *   - `fontWeight`  — `medium` → 510, `semibold`/`bold` → 590, so the spec's
 *     "no weights above 590" rule is enforced by construction.
 *   - `fontSize`    — `xs`/`sm`/`base`/`lg`/`xl`/`2xl` are retuned to the
 *     spec's size + line-height + tracking triples.
 *   - `borderRadius`— `lg`/`xl`/`2xl`/`3xl` all clamp to 12px, the spec's
 *     maximum card radius.
 */

/** `rgb(var(--x) / <alpha-value>)` so opacity modifiers survive. */
const channel = (token: string) => `rgb(var(${token}) / <alpha-value>)`;

const config: Config = {
  darkMode: ["class"],
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}"],
  theme: {
    container: {
      center: true,
      padding: "24px",
      screens: { "2xl": "1200px" },
    },
    extend: {
      colors: {
        /* --- Raw palette (DESIGN.md § Tokens — Colors) -------------------- */
        void: channel("--color-void"),
        carbon: channel("--color-carbon"),
        obsidian: channel("--color-obsidian"),
        graphite: channel("--color-graphite"),
        smoke: channel("--color-smoke"),
        ash: channel("--color-ash"),
        fog: channel("--color-fog"),
        mist: channel("--color-mist"),
        bone: channel("--color-bone"),
        paper: channel("--color-paper"),
        "acid-lime": channel("--color-acid-lime"),
        "pulse-green": channel("--color-pulse-green"),
        "coral-red": channel("--color-coral-red"),
        "signal-teal": channel("--color-signal-teal"),
        "iris-violet": channel("--color-iris-violet"),
        lavender: channel("--color-lavender"),

        /* --- Semantic roles ---------------------------------------------- */
        border: channel("--border"),
        "border-strong": channel("--border-strong"),
        input: channel("--input"),
        ring: channel("--ring"),
        background: channel("--background"),
        foreground: channel("--foreground"),
        /* Headings and max-contrast emphasis. Body copy stays on
           `foreground`; the spec reserves pure white for headings. */
        "foreground-strong": channel("--foreground-strong"),
        primary: {
          DEFAULT: channel("--primary"),
          foreground: channel("--primary-foreground"),
        },
        secondary: {
          DEFAULT: channel("--secondary"),
          foreground: channel("--secondary-foreground"),
        },
        destructive: {
          DEFAULT: channel("--destructive"),
          foreground: channel("--destructive-foreground"),
        },
        muted: {
          DEFAULT: channel("--muted"),
          foreground: channel("--muted-foreground"),
        },
        accent: {
          DEFAULT: channel("--accent"),
          foreground: channel("--accent-foreground"),
        },
        popover: {
          DEFAULT: channel("--popover"),
          foreground: channel("--popover-foreground"),
        },
        card: {
          DEFAULT: channel("--card"),
          foreground: channel("--card-foreground"),
        },
        success: {
          DEFAULT: channel("--success"),
          foreground: channel("--success-foreground"),
        },
        info: channel("--info"),
      },

      fontFamily: {
        sans: ["var(--font-inter-variable)"],
        mono: ["var(--font-berkeley-mono)"],
      },

      /**
       * Size + line-height + tracking arrive as one triple so tracking can
       * never drift from size. `xs`…`2xl` deliberately override the Tailwind
       * defaults; the named roles below are for new work.
       *
       * App chrome lives at 12–15px: DESIGN.md declares "compact" density, so
       * its 16–20px reading sizes are reserved for prose and auth surfaces.
       */
      fontSize: {
        // Retuned defaults — existing call sites inherit spec metrics.
        xs: ["12px", { lineHeight: "1.4" }],
        sm: ["14px", { lineHeight: "1.45", letterSpacing: "-0.011em" }],
        base: ["16px", { lineHeight: "1.5" }],
        lg: ["17px", { lineHeight: "1.6" }],
        xl: ["20px", { lineHeight: "1.33", letterSpacing: "-0.012em" }],
        "2xl": ["24px", { lineHeight: "1.33", letterSpacing: "-0.012em" }],

        // Named roles — DESIGN.md § Type Scale Detail.
        micro: ["10px", { lineHeight: "1.5" }],
        label: ["12px", { lineHeight: "1.4" }],
        caption: ["13px", { lineHeight: "1.2" }],
        "body-sm": ["15px", { lineHeight: "1.6", letterSpacing: "-0.011em" }],
        body: ["16px", { lineHeight: "1.5" }],
        // Weight is NOT bundled here: a fontWeight inside a fontSize entry
        // cannot be overridden by a `font-*` utility, since it lands in the
        // same rule. Pair these with `font-w590` at the call site.
        "body-lg": ["17px", { lineHeight: "1.6" }],
        "body-emphasis": ["20px", { lineHeight: "1.33", letterSpacing: "-0.012em" }],
        heading: ["24px", { lineHeight: "1.33", letterSpacing: "-0.012em" }],
        "heading-sm": ["32px", { lineHeight: "1.13", letterSpacing: "-0.022em" }],
        "heading-lg": ["48px", { lineHeight: "1", letterSpacing: "-0.022em" }],
        hero: ["64px", { lineHeight: "1", letterSpacing: "-0.022em" }],
        display: ["72px", { lineHeight: "1", letterSpacing: "-0.022em" }],
      },

      /**
       * The spec's band is 300–590. `medium`, `semibold` and `bold` are
       * remapped so no utility in the codebase can render above 590 —
       * DESIGN.md § Don't: "Do not use bold weights (700+)".
       */
      fontWeight: {
        light: "300",
        normal: "400",
        medium: "510",
        w510: "510",
        semibold: "590",
        w590: "590",
        bold: "590",
      },

      letterSpacing: {
        tight: "-0.022em",
        snug: "-0.012em",
        normal: "-0.011em",
        mono: "-0.013em",
        wide: "0.02em",
      },

      /**
       * Three radii are the entire vocabulary: 6px controls, 12px surfaces,
       * 9999px pills — plus 2px/4px for the smallest chips. `xl` and above
       * clamp to 12px so oversized surfaces are unreachable.
       */
      borderRadius: {
        none: "0px",
        sm: "2px",
        badge: "4px",
        DEFAULT: "6px",
        md: "6px",
        lg: "12px",
        xl: "12px",
        "2xl": "12px",
        "3xl": "12px",
        full: "9999px",
      },

      /**
       * DESIGN.md § Elevation: hierarchy comes from the surface ladder and
       * hairline borders, not from ambient shadow. `md` is an INSET token per
       * the spec table, so anything needing real elevation uses `xl`.
       */
      boxShadow: {
        sm: "rgba(0, 0, 0, 0.4) 0px 2px 4px 0px",
        md: "rgba(0, 0, 0, 0.2) 0px 0px 12px 0px inset",
        lg: "rgba(8, 9, 10, 0.6) 0px 4px 32px 0px",
        xl: "rgba(8, 9, 10, 0.6) 0px 4px 32px 0px",
        subtle: "rgb(35, 37, 42) 0px 0px 0px 1px inset",
        "subtle-2": "rgba(0, 0, 0, 0.2) 0px 0px 0px 1px",
        "subtle-3":
          "rgba(0, 0, 0, 0.01) 0px 5px 2px 0px, rgba(0, 0, 0, 0.04) 0px 3px 2px 0px, rgba(0, 0, 0, 0.07) 0px 1px 1px 0px, rgba(0, 0, 0, 0.08) 0px 0px 1px 0px",
        "subtle-4":
          "rgba(255, 255, 255, 0.03) 0px 0px 0px 1px inset, rgba(255, 255, 255, 0.04) 0px 1px 0px 0px inset, rgba(0, 0, 0, 0.6) 0px 0px 0px 1px, rgba(0, 0, 0, 0.1) 0px 4px 4px 0px",
        "subtle-5": "rgba(0, 0, 0, 0.1) 0px 0px 0px 2px",
        none: "none",
      },

      /* 0.5px rounds inconsistently at 1x DPI, so app chrome defaults to 1px
         graphite and `border-hairline` stays opt-in for surfaces that want the
         thinner edge on retina. */
      borderWidth: {
        hairline: "0.5px",
      },

      maxWidth: {
        shell: "var(--page-max-width)",
      },

      keyframes: {
        "accordion-down": {
          from: { height: "0" },
          to: { height: "var(--radix-accordion-content-height)" },
        },
        "accordion-up": {
          from: { height: "var(--radix-accordion-content-height)" },
          to: { height: "0" },
        },
      },
      animation: {
        "accordion-down": "accordion-down 0.2s ease-out",
        "accordion-up": "accordion-up 0.2s ease-out",
      },
    },
  },
  plugins: [require("tailwindcss-animate")],
};
export default config;
