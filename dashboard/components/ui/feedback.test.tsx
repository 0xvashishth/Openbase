import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EmptyState, ErrorBanner, SuccessBanner } from "./feedback";

describe("feedback primitives", () => {
  it("EmptyState renders title + hint + action", () => {
    render(<EmptyState title="No projects yet" hint="Create one" action={<button>Create</button>} />);
    expect(screen.getByText("No projects yet")).toBeInTheDocument();
    expect(screen.getByText("Create one")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create" })).toBeInTheDocument();
  });

  it("ErrorBanner uses role=alert and destructive styling", () => {
    render(<ErrorBanner message="Failed to save" />);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Failed to save");
    expect(alert.className).toMatch(/destructive/);
  });

  it("SuccessBanner uses role=status", () => {
    render(<SuccessBanner message="Connected!" />);
    expect(screen.getByRole("status")).toHaveTextContent("Connected!");
  });
});
