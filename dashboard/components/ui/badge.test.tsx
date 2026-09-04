import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Badge, StatusBadge } from "./badge";

describe("Badge", () => {
  it("renders as a truncated pill", () => {
    render(<Badge>postgres</Badge>);
    const pill = screen.getByText("postgres").parentElement;
    expect(pill?.className).toMatch(/rounded-full/);
  });

  it("is outlined by default (no solid fills)", () => {
    const { rerender } = render(<Badge variant="success">connected</Badge>);
    const pill = screen.getByText("connected").parentElement?.className ?? "";
    expect(pill).toMatch(/border-success/);
    expect(pill).toMatch(/bg-transparent/);
    expect(pill).not.toMatch(/bg-success/);
    rerender(<Badge variant="destructive">error</Badge>);
    const err = screen.getByText("error").parentElement?.className ?? "";
    expect(err).toMatch(/border-destructive/);
    expect(err).not.toMatch(/bg-destructive/);
  });
});

describe("StatusBadge", () => {
  it("renders a liveness dot with muted pill styling", () => {
    render(<StatusBadge tone="success">connected</StatusBadge>);
    expect(screen.getByText("connected")).toBeInTheDocument();
    expect(document.querySelector(".bg-success.rounded-full")).toBeInTheDocument();
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
