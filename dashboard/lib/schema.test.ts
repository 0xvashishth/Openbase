import { describe, expect, it } from "vitest";
import { normalizeFullSchema, normalizeNames, normalizeResultSet, normalizeSchemaInfo } from "./schema";

describe("schema normalizers (crash regression)", () => {
  it("normalizeSchemaInfo coerces null columns/indexes to []", () => {
    const out = normalizeSchemaInfo({ collection: "users", columns: null, indexes: null } as never);
    expect(out.columns).toEqual([]);
    expect(out.indexes).toEqual([]);
  });

  it("normalizeFullSchema coerces null collections/relationships (the reported crash)", () => {
    const out = normalizeFullSchema({ collections: null, relationships: null } as never);
    expect(out.collections).toEqual([]);
    expect(out.relationships).toEqual([]);
    // crashed call sites:
    expect(out.collections.length).toBe(0);
    expect(out.relationships.length).toBe(0);
  });

  it("normalizeResultSet coerces null columns/rows", () => {
    const out = normalizeResultSet({ columns: null, rows: null } as never);
    expect(out.columns).toEqual([]);
    expect(out.rows).toEqual([]);
  });

  it("normalizeNames drops non-string entries", () => {
    expect(normalizeNames(null)).toEqual([]);
    expect(normalizeNames([{ name: "a" }, null, {}] as never)).toEqual(["a"]);
  });
});
