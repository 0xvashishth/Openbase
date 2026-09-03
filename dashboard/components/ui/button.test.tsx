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
    expect(screen.getByRole("button").className).toMatch(/border-input/);
  });

  it("supports destructive + outline variants", () => {
    const { rerender } = render(<Button variant="destructive">Delete</Button>);
    expect(screen.getByRole("button").className).toMatch(/bg-destructive/);
    rerender(<Button variant="outline">Cancel</Button>);
    expect(screen.getByRole("button").className).toMatch(/border-input/);
  });

  it("supports asChild via Radix Slot (renders anchor as button)", () => {
    render(
      <Button asChild>
        <a href="/orgs">Go</a>
      </Button>
    );
    const link = screen.getByRole("link", { name: "Go" });
    expect(link).toHaveAttribute("href", "/orgs");
    expect(link.className).toMatch(/border-input/);
  });
});
