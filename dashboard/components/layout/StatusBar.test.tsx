import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StatusBar } from "./StatusBar";

vi.mock("@/lib/api", () => ({ api: { me: vi.fn(), listOrgs: vi.fn() } }));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));

import { api } from "@/lib/api";

describe("StatusBar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.me).mockResolvedValue({} as never);
    vi.mocked(api.listOrgs).mockResolvedValue([] as never);
  });

  it("shows brand top-left linking home", () => {
    render(<StatusBar />);
    expect(screen.getByRole("link", { name: "Openbase home" })).toHaveAttribute("href", "/orgs");
    expect(screen.getByText("Openbase")).toBeInTheDocument();
  });

  it("shows the current org in a badge", () => {
    render(<StatusBar orgName="Acme" />);
    expect(screen.getByText("Acme")).toBeInTheDocument();
  });

  it("shows org skeleton while loading", () => {
    render(<StatusBar orgLoading />);
    expect(screen.getByRole("status", { name: /loading organization/i })).toBeInTheDocument();
  });

  it("shows a live clock with seconds", () => {
    render(<StatusBar />);
    expect(screen.getByRole("timer")).toHaveTextContent(/:\d{2}:\d{2}/);
  });

  it("renders an org switcher button when 2+ orgs exist", async () => {
    vi.mocked(api.listOrgs).mockResolvedValue([
      { id: "o1", name: "Acme" },
      { id: "o2", name: "Beta" },
    ] as never);
    render(<StatusBar orgId="o1" orgName="Acme" />);
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Switch organization" })).toBeInTheDocument();
    });
  });

  it("falls back to a static badge with a single org", async () => {
    vi.mocked(api.listOrgs).mockResolvedValue([{ id: "o1", name: "Acme" }] as never);
    render(<StatusBar orgId="o1" orgName="Acme" />);
    await waitFor(() => {
      expect(screen.getByText("Acme")).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: "Switch organization" })).not.toBeInTheDocument();
  });
});
