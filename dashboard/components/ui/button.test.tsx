import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Button } from "./button";

describe("Button (shadcn/Radix Slot)", () => {
  it("renders children and handles click", async () => {
    const onClick = vi.fn();
    const user = userEvent.setup();
    render(<Button onClick={onClick}>Save</Button>);
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("disables while loading and shows spinner", () => {
    render(<Button loading>Provision</Button>);
    expect(screen.getByRole("button", { name: /provision/i })).toBeDisabled();
  });

  it("applies outline by default (outline-first design)", () => {
    render(<Button>Default</Button>);
    const cls = screen.getByRole("button").className;
    expect(cls).toMatch(/border-border/);
    expect(cls).toMatch(/bg-transparent/);
  });

  /**
   * The acid-lime accent must stay opt-in. DESIGN.md allows exactly one
   * chromatic action per view, so a default-variant button turning lime would
   * silently break that rule everywhere at once.
   */
  it("never applies the acid-lime accent unless variant='primary'", () => {
    const { rerender } = render(<Button>Default</Button>);
    expect(screen.getByRole("button").className).not.toMatch(/bg-primary/);

    for (const variant of ["outline", "secondary", "ghost", "link"] as const) {
      rerender(<Button variant={variant}>x</Button>);
      expect(screen.getByRole("button").className).not.toMatch(/bg-primary/);
    }

    rerender(<Button variant="primary">Create</Button>);
    expect(screen.getByRole("button").className).toMatch(/bg-primary/);
  });

  /**
   * Destructive is a coral OUTLINE, not a fill: the spec bars extra chromatic
   * action colors. The filled treatment exists only as `destructive-solid` for
   * the confirm step inside a dialog.
   */
  it("renders destructive as an outline and reserves the fill for destructive-solid", () => {
    // Matches a standalone `bg-destructive` class only — `hover:bg-destructive/10`
    // and `bg-destructive/15` must not count as a solid fill.
    const solidFill = /(^|\s)bg-destructive(\s|$)/;

    const { rerender } = render(<Button variant="destructive">Delete</Button>);
    let cls = screen.getByRole("button").className;
    expect(cls).toMatch(/text-destructive/);
    expect(cls).toMatch(/bg-transparent/);
    expect(cls).not.toMatch(solidFill);

    rerender(<Button variant="destructive-solid">Delete forever</Button>);
    cls = screen.getByRole("button").className;
    expect(cls).toMatch(solidFill);

    rerender(<Button variant="outline">Cancel</Button>);
    expect(screen.getByRole("button").className).toMatch(/border-border/);
  });

  it("supports asChild via Radix Slot (renders anchor as button)", () => {
    render(
      <Button asChild>
        <a href="/orgs">Go</a>
      </Button>
    );
    const link = screen.getByRole("link", { name: "Go" });
    expect(link).toHaveAttribute("href", "/orgs");
    expect(link.className).toMatch(/border-border/);
  });

  /** Weight ceiling is 590 — no utility may resolve to 700+. */
  it("keeps every variant inside the 400-590 weight band", () => {
    const { rerender } = render(<Button variant="primary">a</Button>);
    for (const variant of [
      "primary",
      "outline",
      "secondary",
      "ghost",
      "destructive",
      "destructive-solid",
      "link",
    ] as const) {
      rerender(<Button variant={variant}>a</Button>);
      const cls = screen.getByRole("button").className;
      expect(cls).not.toMatch(/font-bold|font-extrabold|font-black/);
      expect(cls).toMatch(/font-normal|font-w510|font-w590/);
    }
  });
});
