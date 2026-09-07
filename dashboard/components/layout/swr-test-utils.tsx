import * as React from "react";
import { render as rtlRender, type RenderOptions, type RenderResult } from "@testing-library/react";
import { SWRConfig } from "swr";

/**
 * Test render with an isolated SWR cache per call. The shared hooks in
 * switchers.tsx cache by session key — without isolation, list results leak
 * between tests and order-dependent failures follow.
 */
export function render(ui: React.ReactElement, options?: RenderOptions): RenderResult {
  return rtlRender(<SWRConfig value={{ provider: () => new Map() }}>{ui}</SWRConfig>, options);
}
