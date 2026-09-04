import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach } from "vitest";
import { Topbar } from "./Topbar";
import { api } from "@/lib/api";

let mockPath = "/orgs";

vi.mock("next/navigation", () => ({
  usePathname: () => mockPath,
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("@/components/AuthProvider", () => ({
  useAuth: () => ({ user: { email: "a@b.c" }, logout: vi.fn(), loading: false }),
  authToken: () => "tok",
}));

vi.mock("@/lib/api", () => ({
  api: { listOrgs: vi.fn(), listProjects: vi.fn(), listCollections: vi.fn() },
}));

describe("Topbar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listOrgs).mockResolvedValue([]);
    vi.mocked(api.listProjects).mockResolvedValue([]);
  });
  it("shows platform breadcrumb + search trigger (brand lives in the status bar)", () => {
    mockPath = "/orgs";
    render(<Topbar />);
    expect(screen.getByRole("navigation", { name: "Breadcrumb" })).toHaveTextContent("Organizations");
    expect(screen.queryByRole("link", { name: "Openbase home" })).not.toBeInTheDocument();
    // desktop full trigger + mobile icon trigger (CSS hides one; jsdom sees both)
    expect(screen.getAllByRole("button", { name: /search/i })).toHaveLength(2);
  });

  it("renders org breadcrumb with link back to orgs", () => {
    mockPath = "/orgs/o1";
    render(<Topbar orgName="Acme" />);
    expect(screen.getByRole("link", { name: "Organizations" })).toHaveAttribute("href", "/orgs");
    expect(screen.getByText("Acme")).toHaveAttribute("aria-current", "page");
  });

  it("renders full project trail with tool label", () => {
    mockPath = "/orgs/o1/projects/p1/sql";
    render(<Topbar orgName="Acme" projectName="Shop" />);
    expect(screen.getByRole("link", { name: "Acme" })).toHaveAttribute("href", "/orgs/o1");
    expect(screen.getByRole("link", { name: "Shop" })).toHaveAttribute(
      "href",
      "/orgs/o1/projects/p1"
    );
    expect(screen.getByText("SQL Editor")).toHaveAttribute("aria-current", "page");
  });

  it("omits the tool crumb on project overview", () => {
    mockPath = "/orgs/o1/projects/p1";
    render(<Topbar orgName="Acme" projectName="Shop" />);
    expect(screen.getByText("Shop")).toHaveAttribute("aria-current", "page");
    expect(screen.queryByText("Overview")).not.toBeInTheDocument();
  });

  it("shows engine + connection badges under breadcrumbs once settled", () => {
    mockPath = "/orgs/o1/projects/p1/tables";
    render(<Topbar orgName="Acme" projectName="Shop" engine="postgres" connected />);
    expect(screen.getByText("postgres")).toBeInTheDocument();
    expect(screen.getByText("connected")).toBeInTheDocument();
  });

  it("shows badge skeletons — never a premature not-connected — while loading", () => {
    mockPath = "/orgs/o1/projects/p1/tables";
    render(<Topbar orgName="Acme" projectName="Shop" projectLoading engine="postgres" connected={false} />);
    expect(screen.queryByText("not connected")).not.toBeInTheDocument();
    expect(screen.queryByText("connected")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Connection status")).toBeInTheDocument();
  });

  it("shows not-connected badge when settled without a connection", () => {
    mockPath = "/orgs/o1/projects/p1/tables";
    render(<Topbar orgName="Acme" projectName="Shop" connected={false} />);
    expect(screen.getByText("not connected")).toBeInTheDocument();
  });

  it("shows org + project switchers when multiple orgs/projects exist", async () => {
    mockPath = "/orgs/o1/projects/p1/tables";
    vi.mocked(api.listOrgs).mockResolvedValue([
      { id: "o1", name: "Acme" },
      { id: "o2", name: "Beta" },
    ] as never);
    vi.mocked(api.listProjects).mockResolvedValue([
      { id: "p1", name: "Shop" },
      { id: "p2", name: "Blog" },
    ] as never);
    render(<Topbar orgName="Acme" projectName="Shop" />);
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Switch organization" })).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Switch project" })).toBeInTheDocument();
  });

  it("omits switchers when only a single org/project exists", async () => {
    mockPath = "/orgs/o1/projects/p1/tables";
    vi.mocked(api.listOrgs).mockResolvedValue([{ id: "o1", name: "Acme" }] as never);
    vi.mocked(api.listProjects).mockResolvedValue([{ id: "p1", name: "Shop" }] as never);
    render(<Topbar orgName="Acme" projectName="Shop" />);
    // let any async fetch settle
    await waitFor(() => {
      expect(screen.getByText("Shop")).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: "Switch organization" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Switch project" })).not.toBeInTheDocument();
  });
});
