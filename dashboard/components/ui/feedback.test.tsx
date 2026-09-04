import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EmptyState, ErrorBanner, PageHeader, SuccessBanner } from "./feedback";

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

  it("PageHeader renders title + info tooltip instead of inline subtitle", () => {
    render(<PageHeader title="Organizations" subtitle="Everything lives inside an org." />);
    expect(screen.getByRole("heading", { name: /Organizations/ })).toBeInTheDocument();
    // description moved to info icon tooltip, not inline text
    expect(screen.queryByText("Everything lives inside an org.")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "About Organizations" })).toBeInTheDocument();
  });
});
