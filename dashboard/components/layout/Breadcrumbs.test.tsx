import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Breadcrumbs, toolDescription, toolLabel } from "./Breadcrumbs";

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

  it("labels the renamed DB Source tab (and its legacy slug)", () => {
    expect(toolLabel("db-source")).toBe("DB Source");
    expect(toolLabel("connection")).toBe("DB Source");
  });

  it("labels Connect separately from DB Source", () => {
    expect(toolLabel("connect")).toBe("Connect");
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

  it("renders a trailing info button when info is provided", () => {
    render(<Breadcrumbs items={[{ label: "Tables" }]} info="Browse collections and rows." />);
    expect(screen.getByRole("button", { name: "About Tables" })).toBeInTheDocument();
  });

  it("omits the info button when info is absent", () => {
    render(<Breadcrumbs items={[{ label: "Tables" }]} />);
    expect(screen.queryByRole("button", { name: "About Tables" })).not.toBeInTheDocument();
  });
});

describe("toolDescription", () => {
  it("maps tool slugs and aliases to descriptions", () => {
    expect(toolDescription("tables")).toMatch(/browse collections/i);
    expect(toolDescription("data")).toMatch(/browse collections/i);
    expect(toolDescription("api-keys")).toMatch(/rest api/i);
    expect(toolDescription("db-source")).toMatch(/provision a database/i);
    expect(toolDescription("connection")).toMatch(/provision a database/i);
    expect(toolDescription("connect")).toMatch(/connect your app/i);
    expect(toolDescription(null)).toMatch(/status and shortcuts/i);
  });

  it("returns undefined for unknown tools", () => {
    expect(toolDescription("billing")).toBeUndefined();
  });
});
