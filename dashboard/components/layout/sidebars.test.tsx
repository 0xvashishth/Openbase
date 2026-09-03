import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PlatformSidebar } from "./PlatformSidebar";
import { OrgSidebar } from "./OrgSidebar";
import { ProjectSidebar } from "./ProjectSidebar";

describe("PlatformSidebar (no org selected)", () => {
  it("shows org/project entry points and NO database tools", () => {
    render(<PlatformSidebar currentPath="/orgs" />);
    expect(screen.getByText("Organizations")).toBeInTheDocument();
    expect(screen.getByText("All projects")).toBeInTheDocument();
    expect(screen.queryByText("Tables")).not.toBeInTheDocument();
    expect(screen.queryByText("Schema")).not.toBeInTheDocument();
    expect(screen.queryByText("Triggers")).not.toBeInTheDocument();
    expect(screen.queryByText("Realtime")).not.toBeInTheDocument();
    expect(screen.queryByText("Connection")).not.toBeInTheDocument();
  });
});

describe("OrgSidebar (org selected, no project)", () => {
  it("shows org-level nav and NO project DB tools", () => {
    render(<OrgSidebar orgId="org-1" orgName="Acme" currentPath="/orgs/org-1" />);
    expect(screen.getByText("Projects")).toBeInTheDocument();
    expect(screen.getByText("Members")).toBeInTheDocument();
    expect(screen.getByText("Settings")).toBeInTheDocument();
    expect(screen.queryByText("Tables")).not.toBeInTheDocument();
    expect(screen.queryByText("API Keys")).not.toBeInTheDocument();
    expect(screen.queryByText("Connection")).not.toBeInTheDocument();
  });

  it("offers a back link to all organizations", () => {
    render(<OrgSidebar orgId="org-1" currentPath="/orgs/org-1" />);
    expect(screen.getByText("All organizations")).toBeInTheDocument();
  });
});

describe("ProjectSidebar (project selected)", () => {
  const base = {
    orgId: "o1",
    projectId: "p1",
    projectName: "Shop",
    engine: "postgres",
  };

  it("groups tools and marks the active tool", () => {
    render(<ProjectSidebar {...base} currentPath="/orgs/o1/projects/p1/tables" />);
    expect(screen.getByText("Database")).toBeInTheDocument();
    expect(screen.getByText("Backend")).toBeInTheDocument();
    expect(screen.getByText("Configure")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /tables/i })).toHaveAttribute("aria-current", "page");
  });

  it("locks gated tools when no database is connected", () => {
    render(<ProjectSidebar {...base} hasConnection={false} currentPath="/orgs/o1/projects/p1" />);
    // Tables renders as disabled span, not a link
    expect(screen.getByText("Tables").closest("a")).toBeNull();
    expect(screen.getByText("Tables").getAttribute("title")).toMatch(/connect/i);
  });

  it("disables triggers/realtime for engines without native support", () => {
    render(
      <ProjectSidebar
        {...base}
        hasConnection
        connected
        supportsTriggers={false}
        supportsRealtime={false}
        currentPath="/orgs/o1/projects/p1"
      />
    );
    expect(screen.getByText("Triggers").getAttribute("title")).toMatch(/trigger/i);
    expect(screen.getByText("Realtime").getAttribute("title")).toMatch(/realtime/i);
  });

  it("shows engine + connection badges", () => {
    render(<ProjectSidebar {...base} connected engine="postgres" currentPath="/orgs/o1/projects/p1" />);
    expect(screen.getByText("postgres")).toBeInTheDocument();
    expect(screen.getByText("connected")).toBeInTheDocument();
  });
});
