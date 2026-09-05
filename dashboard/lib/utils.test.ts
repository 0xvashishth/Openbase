import { describe, expect, it } from "vitest";
import { cn } from "./utils";

describe("cn", () => {
  it("merges class names", () => {
    expect(cn("px-2", "py-1")).toBe("px-2 py-1");
  });

  it("resolves tailwind conflicts (last wins)", () => {
    expect(cn("px-2", "px-4")).toBe("px-4");
  });

  it("handles conditionals", () => {
    expect(cn("base", false && "hidden", undefined, "extra")).toBe("base extra");
  });

  /**
   * Design-system scales are custom, so tailwind-merge has to be taught them.
   * Without that, an unknown `text-*` is treated as a COLOR and one of these
   * two classes gets silently dropped — which is exactly how variant base
   * classes lose their color (or their size) at a call site.
   */
  describe("custom design-system scales", () => {
    it("keeps font-size and text-color together in both orders", () => {
      expect(cn("text-caption", "text-foreground")).toBe("text-caption text-foreground");
      expect(cn("text-foreground", "text-caption")).toBe("text-foreground text-caption");
    });

    it("treats custom font sizes as one conflict group", () => {
      expect(cn("text-caption", "text-label")).toBe("text-label");
      expect(cn("text-sm", "text-caption")).toBe("text-caption");
    });

    it("conflicts w510/w590 with the built-in weights, not with the font family", () => {
      expect(cn("font-normal", "font-w510")).toBe("font-w510");
      expect(cn("font-w510", "font-semibold")).toBe("font-semibold");
      expect(cn("font-mono", "font-w510")).toBe("font-mono font-w510");
    });

    it("resolves custom radius, max-width and border-width values", () => {
      expect(cn("rounded-md", "rounded-badge")).toBe("rounded-badge");
      expect(cn("max-w-xl", "max-w-shell")).toBe("max-w-shell");
      expect(cn("border", "border-hairline")).toBe("border-hairline");
    });
  });
});
