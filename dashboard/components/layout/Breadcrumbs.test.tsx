import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Breadcrumbs, toolLabel } from "./Breadcrumbs";

describe("toolLabel", () => {
  it("maps slugs and legacy aliases to display names", () => {
    expect(toolLabel("overview")).toBe("Overview");
    expect(toolLabel("tables")).toBe("Tables");
    expect(toolLabel("data")).toBe("Tables");
    expect(toolLabel("api-keys")).toBe("API Keys");
    expect(toolLabel("sql")).toBe("SQL Editor");
    expect(toolLabel("members")).toBe("Members");
    expect(toolLabel(null)).toBe("Overview");
  });

  it("capitalizes unknown tools", () => {
    expect(toolLabel("billing")).toBe("Billing");
  });
});

describe("Breadcrumbs", () => {
  it("links ancestors and marks current page", () => {
    render(
      <Breadcrumbs
        items={[
          { label: "Organizations", href: "/orgs" },
          { label: "Acme", href: "/orgs/o1" },
          { label: "Tables" },
        ]}
      />
    );
    expect(screen.getByRole("link", { name: "Organizations" })).toHaveAttribute("href", "/orgs");
    expect(screen.getByRole("link", { name: "Acme" })).toHaveAttribute("href", "/orgs/o1");
    const current = screen.getByText("Tables");
    expect(current).toHaveAttribute("aria-current", "page");
    expect(current.tagName).not.toBe("A");
  });

  it("shows loading skeletons with accessible labels", () => {
    render(<Breadcrumbs items={[{ label: "Organization", loading: true }, { label: "Projects" }]} />);
    expect(screen.getByRole("status", { name: /loading organization/i })).toBeInTheDocument();
  });

  it("exposes a breadcrumb landmark", () => {
    render(<Breadcrumbs items={[{ label: "Organizations" }]} />);
    expect(screen.getByRole("navigation", { name: "Breadcrumb" })).toBeInTheDocument();
  });

  it("renders a shadcn switcher button when 2+ dropdown items exist", () => {
    render(
      <Breadcrumbs
        items={[
          { label: "Organizations", href: "/orgs" },
          {
            label: "Acme",
            href: "/orgs/o1",
            dropdownItems: [
              { label: "Acme", href: "/orgs/o1", current: true },
              { label: "Beta", href: "/orgs/o2" },
            ],
            dropdownLabel: "Switch organization",
          },
          { label: "Tables" },
        ]}
      />
    );
    // base link preserved for navigation
    expect(screen.getByRole("link", { name: "Acme" })).toHaveAttribute("href", "/orgs/o1");
    expect(screen.getByRole("button", { name: "Switch organization" })).toBeInTheDocument();
  });

  it("omits the switcher button when only one dropdown item exists", () => {
    render(
      <Breadcrumbs
        items={[
          {
            label: "Acme",
            dropdownItems: [{ label: "Acme", href: "/orgs/o1", current: true }],
            dropdownLabel: "Switch organization",
          },
        ]}
      />
    );
    expect(screen.queryByRole("button", { name: "Switch organization" })).not.toBeInTheDocument();
  });
});
