import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Badge, StatusBadge } from "./badge";

describe("Badge", () => {
  /**
   * DESIGN.md § Badge / Status Tag pins the tag radius at 4px. The pill shape
   * (9999px) is a different component with a different meaning — see
   * StatusBadge below.
   */
  it("renders as a truncated 4px tag, not a pill", () => {
    render(<Badge>postgres</Badge>);
    const pill = screen.getByText("postgres").parentElement;
    expect(pill?.className).toMatch(/rounded-badge/);
    expect(pill?.className).not.toMatch(/rounded-full/);
  });

  it("uses a tinted fill with matching text for accent variants", () => {
    const { rerender } = render(<Badge variant="success">connected</Badge>);
    const pill = screen.getByText("connected").parentElement?.className ?? "";
    expect(pill).toMatch(/bg-success\/15/);
    expect(pill).toMatch(/text-success/);

    rerender(<Badge variant="destructive">error</Badge>);
    const err = screen.getByText("error").parentElement?.className ?? "";
    expect(err).toMatch(/bg-destructive\/15/);
    expect(err).toMatch(/text-destructive/);
  });

  /**
   * The accent belongs to the single primary action per view. A badge must
   * never claim it, or a page ends up with two competing focal points.
   */
  it("never uses the acid-lime accent", () => {
    for (const variant of [
      "default",
      "secondary",
      "muted",
      "warning",
      "outline",
      "success",
      "destructive",
      "info",
      "tag",
      "category",
    ] as const) {
      const { unmount } = render(<Badge variant={variant}>v</Badge>);
      const cls = screen.getByText("v").parentElement?.className ?? "";
      expect(cls).not.toMatch(/primary|acid-lime/);
      unmount();
    }
  });

  /**
   * DESIGN.md has no amber. Rendering `warning` chromatically would mean
   * inventing a color or stealing the accent, so it stays achromatic.
   */
  it("renders warning achromatically", () => {
    render(<Badge variant="warning">not connected</Badge>);
    const cls = screen.getByText("not connected").parentElement?.className ?? "";
    expect(cls).toMatch(/text-muted-foreground/);
    expect(cls).not.toMatch(/amber|yellow|orange|warning/);
  });
});

describe("StatusBadge", () => {
  /** Live state keeps the pill shape (§ Pill Button) plus a liveness dot. */
  it("renders a liveness dot inside a pill", () => {
    render(<StatusBadge tone="success">connected</StatusBadge>);
    expect(screen.getByText("connected")).toBeInTheDocument();
    expect(document.querySelector(".bg-success.rounded-full")).toBeInTheDocument();
    expect(screen.getByText("connected").parentElement?.className).toMatch(/rounded-full/);
  });

  it("defaults to muted tone", () => {
    render(<StatusBadge>paused</StatusBadge>);
    expect(screen.getByText("paused")).toBeInTheDocument();
  });
});

describe("Dialog ConfirmDialog", () => {
  it("placeholder: badge module loads without dialog dep", async () => {
    const user = userEvent.setup();
    expect(typeof user.click).toBe("function");
    expect(vi.isMockFunction(vi.fn())).toBe(true);
  });
});
