import { EditorView } from "@codemirror/view";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags as t } from "@lezer/highlight";
import type { Extension } from "@codemirror/state";

/**
 * CodeMirror themes built on the Openbase design system (DESIGN.md).
 *
 * Replaces `oneDark`, which shipped its own unrelated palette (#282c34 surface,
 * purple/red syntax) — an editor pane in a completely different color system
 * sitting inside the dashboard.
 *
 * Palette values are the literal spec hex codes rather than `var(--token)`:
 * CodeMirror injects these through a StyleModule at document level, and
 * resolving custom properties there is unreliable across the theme swap. Two
 * explicit themes, selected by resolvedTheme, is the honest approach.
 *
 * Syntax coloring is deliberately restrained. The spec's chromatic accents are
 * "supporting" and acid lime is reserved for the primary action, so code reads
 * mostly in the grey ramp with Signal Teal for keywords, Iris Violet for
 * strings and Lavender for numbers — enough differentiation to parse structure
 * without turning the editor into the loudest element on the page.
 */

const VOID = "#08090a";
const CARBON = "#0f1011";
const GRAPHITE = "#23252a";
const SMOKE = "#383b3f";
const ASH = "#62666d";
const FOG = "#8a8f98";
const MIST = "#d0d6e0";
const PAPER = "#ffffff";
const TEAL = "#02b8cc";
const IRIS = "#6366f1";
const LAVENDER = "#8b5cf6";
const CORAL = "#eb5757";

/** Light-mode values mirror app/globals.css, including its AA-driven shifts. */
const L_BG = "#ffffff";
const L_SURFACE = "#fbfbfc";
const L_BORDER = "#e5e6e8";
const L_TEXT = "#3c4149";
const L_MUTED = "#62666d";
const L_TEAL = "#0291a1";
const L_IRIS = "#4f46e5";
const L_LAVENDER = "#7c3aed";
const L_CORAL = "#d92d2d";

function buildTheme({
  dark,
  bg,
  surface,
  border,
  text,
  muted,
  faint,
  caret,
  selection,
}: {
  dark: boolean;
  bg: string;
  surface: string;
  border: string;
  text: string;
  muted: string;
  faint: string;
  caret: string;
  selection: string;
}): Extension {
  return EditorView.theme(
    {
      "&": {
        // 13px matches the app's caption scale; the editor is chrome-adjacent,
        // not a reading surface.
        fontSize: "13px",
        backgroundColor: bg,
        color: text,
      },
      ".cm-content": {
        fontFamily: "var(--font-berkeley-mono)",
        caretColor: caret,
        padding: "8px 0",
      },
      ".cm-scroller": {
        fontFamily: "var(--font-berkeley-mono)",
        lineHeight: "1.7",
      },
      "&.cm-focused": { outline: "none" },
      ".cm-cursor, .cm-dropCursor": { borderLeftColor: caret, borderLeftWidth: "1.5px" },
      "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection": {
        backgroundColor: selection,
      },
      ".cm-gutters": {
        backgroundColor: surface,
        color: faint,
        border: "none",
        borderRight: `1px solid ${border}`,
      },
      ".cm-activeLineGutter": { backgroundColor: "transparent", color: muted },
      ".cm-activeLine": { backgroundColor: dark ? "rgba(255,255,255,0.02)" : "rgba(8,9,10,0.02)" },
      ".cm-lineNumbers .cm-gutterElement": { padding: "0 8px 0 12px", minWidth: "32px" },
      ".cm-placeholder": { color: faint },
      ".cm-selectionMatch": { backgroundColor: dark ? "rgba(2,184,204,0.15)" : "rgba(2,145,161,0.12)" },
      ".cm-matchingBracket, .cm-nonmatchingBracket": {
        backgroundColor: "transparent",
        outline: `1px solid ${muted}`,
      },
      ".cm-tooltip": {
        backgroundColor: dark ? "#161718" : L_BG,
        border: `1px solid ${border}`,
        borderRadius: "6px",
        color: text,
      },
      ".cm-tooltip-autocomplete > ul > li[aria-selected]": {
        backgroundColor: dark ? GRAPHITE : "#f4f5f7",
        color: dark ? PAPER : VOID,
      },
    },
    { dark }
  );
}

function buildHighlight(c: {
  keyword: string;
  string: string;
  number: string;
  comment: string;
  name: string;
  type: string;
  invalid: string;
  text: string;
}): Extension {
  return syntaxHighlighting(
    HighlightStyle.define([
      { tag: t.keyword, color: c.keyword },
      { tag: [t.operatorKeyword, t.modifier], color: c.keyword },
      { tag: [t.controlKeyword, t.definitionKeyword], color: c.keyword },
      { tag: [t.string, t.special(t.string)], color: c.string },
      { tag: [t.number, t.bool, t.null], color: c.number },
      { tag: [t.comment, t.lineComment, t.blockComment], color: c.comment, fontStyle: "italic" },
      { tag: [t.variableName, t.propertyName], color: c.text },
      { tag: [t.function(t.variableName), t.function(t.propertyName)], color: c.name },
      { tag: [t.typeName, t.className, t.namespace], color: c.type },
      { tag: [t.operator, t.punctuation, t.separator, t.bracket], color: c.comment },
      { tag: t.invalid, color: c.invalid },
      { tag: t.strong, fontWeight: "590" },
      { tag: t.emphasis, fontStyle: "italic" },
      { tag: t.link, color: c.name, textDecoration: "underline" },
    ])
  );
}

/** Dark editor theme — the canonical DESIGN.md surface ladder. */
export const openbaseDark: Extension = [
  buildTheme({
    dark: true,
    bg: CARBON,
    surface: CARBON,
    border: GRAPHITE,
    text: MIST,
    muted: FOG,
    faint: ASH,
    caret: MIST,
    selection: "rgba(208,214,224,0.14)",
  }),
  buildHighlight({
    keyword: TEAL,
    string: IRIS,
    number: LAVENDER,
    comment: ASH,
    name: MIST,
    type: TEAL,
    invalid: CORAL,
    text: MIST,
  }),
];

/** Light editor theme — derived, mirroring globals.css. */
export const openbaseLight: Extension = [
  buildTheme({
    dark: false,
    bg: L_SURFACE,
    surface: L_SURFACE,
    border: L_BORDER,
    text: L_TEXT,
    muted: L_MUTED,
    faint: "#8a8f98",
    caret: VOID,
    selection: "rgba(8,9,10,0.10)",
  }),
  buildHighlight({
    keyword: L_TEAL,
    string: L_IRIS,
    number: L_LAVENDER,
    comment: SMOKE,
    name: L_TEXT,
    type: L_TEAL,
    invalid: L_CORAL,
    text: L_TEXT,
  }),
];
