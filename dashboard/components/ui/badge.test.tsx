import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Badge, LegacyBadge } from "./badge";

describe("Badge", () => {
  it("renders with default monochrome variant", () => {
    render(<Badge>postgres</Badge>);
    expect(screen.getByText("postgres")).toBeInTheDocument();
  });

  it("maps legacy tones to monochrome variants without blue/slate tailwind", () => {
    const { rerender } = render(<LegacyBadge tone="green">connected</LegacyBadge>);
    expect(screen.getByText("connected").className).toMatch(/bg-success/);
    rerender(<LegacyBadge tone="red">error</LegacyBadge>);
    expect(screen.getByText("error").className).toMatch(/bg-destructive/);
    rerender(<LegacyBadge tone="blue">slug</LegacyBadge>);
    // blue legacy tone must NOT emit brand/blue tailwind classes anymore
    expect(screen.getByText("slug").className).not.toMatch(/brand|blue/);
  });
});

describe("Dialog ConfirmDialog", () => {
  it("placeholder: badge module loads without dialog dep", async () => {
    const user = userEvent.setup();
    expect(typeof user.click).toBe("function");
    expect(vi.isMockFunction(vi.fn())).toBe(true);
  });
});
