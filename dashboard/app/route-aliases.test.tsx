import { describe, expect, it, vi } from "vitest";

const redirect = vi.fn();
vi.mock("next/navigation", () => ({ redirect: (url: string) => redirect(url) }));

import ConnectionAlias from "@/app/(app)/orgs/[orgId]/projects/[projectId]/connection/page";
import ApiKeysAlias from "@/app/(app)/orgs/[orgId]/projects/[projectId]/api-keys/page";
import DataAlias from "@/app/(app)/orgs/[orgId]/projects/[projectId]/data/page";

/**
 * The renamed/legacy tool slugs must keep resolving so existing bookmarks and
 * links survive the Connection → DB Source rename.
 */
describe("legacy tool route aliases", () => {
  it.each([
    ["connection → db-source", ConnectionAlias, "/orgs/o1/projects/p1/db-source"],
    ["api-keys → api", ApiKeysAlias, "/orgs/o1/projects/p1/api"],
    ["data → tables", DataAlias, "/orgs/o1/projects/p1/tables"],
  ])("redirects %s", async (_name, Page, expected) => {
    redirect.mockClear();
    await Page({ params: Promise.resolve({ orgId: "o1", projectId: "p1" }) });
    expect(redirect).toHaveBeenCalledWith(expected);
  });
});
