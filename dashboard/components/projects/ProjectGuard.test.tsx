import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ProjectGuard } from "./ProjectGuard";

const mockUseProject = vi.fn();

vi.mock("@/lib/project-context", () => ({
  useProject: () => mockUseProject(),
}));

describe("ProjectGuard", () => {
  it("shows skeleton while loading", () => {
    mockUseProject.mockReturnValue({ loading: true });
    render(
      <ProjectGuard>
        <p>child</p>
      </ProjectGuard>
    );
    expect(screen.queryByText("child")).not.toBeInTheDocument();
  });

  it("blocks connection-gated tools when not connected with CTA", () => {
    mockUseProject.mockReturnValue({
      loading: false,
      error: null,
      hasConnection: false,
      orgId: "o1",
      projectId: "p1",
    });
    render(
      <ProjectGuard requireConnection toolName="Tables">
        <p>child</p>
      </ProjectGuard>
    );
    expect(screen.getByText(/connect a database first/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /go to connection/i })).toHaveAttribute(
      "href",
      "/orgs/o1/projects/p1/connection"
    );
  });

  it("blocks triggers when engine lacks native support", () => {
    mockUseProject.mockReturnValue({
      loading: false,
      error: null,
      hasConnection: true,
      supportsTriggers: false,
    });
    render(
      <ProjectGuard requireConnection requireTriggers>
        <p>child</p>
      </ProjectGuard>
    );
    expect(screen.getByText(/native triggers not supported/i)).toBeInTheDocument();
  });

  it("blocks realtime when engine lacks native support", () => {
    mockUseProject.mockReturnValue({
      loading: false,
      error: null,
      hasConnection: true,
      supportsRealtime: false,
    });
    render(
      <ProjectGuard requireConnection requireRealtime>
        <p>child</p>
      </ProjectGuard>
    );
    expect(screen.getByText(/native realtime not supported/i)).toBeInTheDocument();
  });

  it("renders children when all gates pass", () => {
    mockUseProject.mockReturnValue({
      loading: false,
      error: null,
      hasConnection: true,
      supportsTriggers: true,
      supportsRealtime: true,
    });
    render(
      <ProjectGuard requireConnection requireTriggers requireRealtime>
        <p>guarded content</p>
      </ProjectGuard>
    );
    expect(screen.getByText("guarded content")).toBeInTheDocument();
  });
});
