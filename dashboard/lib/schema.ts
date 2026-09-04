import type { FullSchema, ResultSet, SchemaInfo } from "./types";

/**
 * Backend is Go: nil slices serialize as JSON `null`, not `[]`.
 * Every consumer that calls `.length` / `.map` MUST go through these
 * normalizers first — this is the fix for the schema-visualisation crash
 * (`Cannot read properties of null (reading 'length')`).
 */
export function normalizeSchemaInfo(s: SchemaInfo | null | undefined): SchemaInfo {
  // Backend `/schema` historically returned `{name}` while `/collections/{id}`
  // returns `{collection}` — accept both so the explorer never shows "unknown".
  const raw = s as (Partial<SchemaInfo> & { name?: unknown }) | null | undefined;
  const collection =
    typeof raw?.collection === "string" && raw.collection
      ? raw.collection
      : typeof raw?.name === "string" && raw.name
        ? raw.name
        : "unknown";
  return {
    collection,
    columns: Array.isArray(s?.columns) ? (s!.columns as SchemaInfo["columns"]) : [],
    indexes: Array.isArray(s?.indexes) ? (s!.indexes as SchemaInfo["indexes"]) : [],
  };
}

export function normalizeResultSet(r: ResultSet | null | undefined): ResultSet {
  return {
    columns: Array.isArray(r?.columns) ? (r!.columns as string[]) : [],
    rows: Array.isArray(r?.rows) ? (r!.rows as ResultSet["rows"]) : [],
  };
}

export function normalizeFullSchema(s: FullSchema | null | undefined): FullSchema {
  return {
    collections: Array.isArray(s?.collections)
      ? (s!.collections as SchemaInfo[]).map(normalizeSchemaInfo)
      : [],
    relationships: Array.isArray(s?.relationships) ? (s!.relationships as FullSchema["relationships"]) : [],
    capabilities: s?.capabilities ?? {
      supports_relational_joins: false,
      supports_foreign_keys: false,
      supports_native_triggers: false,
      supports_change_streams: false,
      supports_realtime: "none",
      supports_transactions: false,
      supports_full_text_search: false,
      supports_vector_search: false,
    },
  };
}

export function normalizeNames(
  cols: { name: string }[] | null | undefined
): string[] {
  if (!Array.isArray(cols)) return [];
  return cols.map((c) => c?.name).filter((n): n is string => typeof n === "string");
}
