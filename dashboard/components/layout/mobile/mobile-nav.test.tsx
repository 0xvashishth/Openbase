import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi, beforeEach } from "vitest";
import * as React from "react";
import { render } from "../swr-test-utils";
import { MobileBottomNav } from "./MobileBottomNav";
import { MobileTopbar } from "./MobileTopbar";
import { MobileSheet, MobileSheetProvider, useMobileSheet } from "./MobileSheet";
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

/** Opens the drawer on mount so content assertions don't depend on animation timing. */
function OpenDrawer() {
  const { setOpen } = useMobileSheet();
  React.useEffect(() => {
    setOpen(true);
  }, [setOpen]);
  return null;
}

describe("MobileBottomNav", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listOrgs).mockResolvedValue([]);
    vi.mocked(api.listProjects).mockResolvedValue([]);
  });

  it("is mobile-only (hidden at md and up, desktop untouched)", () => {
    mockPath = "/orgs";
    render(<MobileBottomNav />);
    expect(screen.getByRole("navigation", { name: "Primary navigation" })).toHaveClass("md:hidden");
  });

  it("shows platform destinations and marks the active one", () => {
    mockPath = "/orgs";
    render(<MobileBottomNav />);
    expect(screen.getByRole("link", { name: "Orgs" })).toHaveAttribute("href", "/orgs");
    expect(screen.getByRole("link", { name: "Orgs" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Projects" })).toHaveAttribute("href", "/projects");
    expect(screen.getByRole("link", { name: "Account" })).toHaveAttribute("href", "/account");
  });

  it("shows org destinations inside org scope", () => {
    mockPath = "/orgs/o1";
    render(<MobileBottomNav />);
    expect(screen.getByRole("link", { name: "Projects" })).toHaveAttribute("href", "/orgs/o1");
    expect(screen.getByRole("link", { name: "Members" })).toHaveAttribute("href", "/orgs/o1/members");
    expect(screen.getByRole("link", { name: "Settings" })).toHaveAttribute("href", "/orgs/o1/settings");
  });

  it("shows primary project tools with a More overflow menu", async () => {
    mockPath = "/orgs/o1/projects/p1/sql";
    render(<MobileBottomNav />);
    expect(screen.getByRole("link", { name: "Overview" })).toHaveAttribute("href", "/orgs/o1/projects/p1");
    const sql = screen.getByRole("link", { name: "SQL" });
    expect(sql).toHaveAttribute("href", "/orgs/o1/projects/p1/sql");
    expect(sql).toHaveAttribute("aria-current", "page");
    // API Keys overflows into the More menu
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "More" }));
    await waitFor(() => {
      expect(screen.getAllByRole("menuitem").length).toBeGreaterThan(0);
    });
  });
});

describe("MobileTopbar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listOrgs).mockResolvedValue([]);
    vi.mocked(api.listProjects).mockResolvedValue([]);
  });

  it("is mobile-only and exposes menu, search and account controls", () => {
    mockPath = "/orgs";
    render(
      <MobileSheetProvider>
        <MobileTopbar />
      </MobileSheetProvider>
    );
    expect(screen.getByRole("banner").className).toMatch(/md:hidden/);
    expect(screen.getByRole("button", { name: "Open menu" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /Search/ }).length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Account menu" })).toBeInTheDocument();
  });
});

describe("MobileSheet", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listOrgs).mockResolvedValue([]);
    vi.mocked(api.listProjects).mockResolvedValue([]);
  });

  it("stays closed until the trigger opens it", async () => {
    mockPath = "/orgs";
    render(
      <MobileSheetProvider>
        <MobileSheet />
      </MobileSheetProvider>
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("lists workspace sections when open in platform scope", async () => {
    mockPath = "/orgs";
    render(
      <MobileSheetProvider>
        <OpenDrawer />
        <MobileSheet />
      </MobileSheetProvider>
    );
    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });
    expect(screen.getByRole("link", { name: "Organizations" })).toHaveAttribute("href", "/orgs");
    expect(screen.getByRole("link", { name: "All projects" })).toHaveAttribute("href", "/projects");
  });

  it("lists org sections with a back link when open in org scope", async () => {
    mockPath = "/orgs/o1/members";
    render(
      <MobileSheetProvider>
        <OpenDrawer />
        <MobileSheet />
      </MobileSheetProvider>
    );
    await waitFor(() => {
      expect(screen.getByRole("link", { name: "All organizations" })).toHaveAttribute("href", "/orgs");
    });
    expect(screen.getByRole("link", { name: "Members" })).toHaveAttribute("aria-current", "page");
  });
});
