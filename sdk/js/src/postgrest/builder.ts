// PostgREST-compatible query builder. Serializes the full filter/operator
// grammar (Phase 12.1) so the same SDK call works once the backend lands,
// while staying compatible with today's backend (`limit/offset/order_by/order`).

import { Transport, type FetchImpl } from '../lib/fetch.js'

export type CountOption = 'exact' | 'planned' | 'estimated'
export type FilterOperator =
  | 'eq' | 'neq' | 'gt' | 'gte' | 'lt' | 'lte'
  | 'like' | 'ilike' | 'is' | 'in'
  | 'cs' | 'cd' | 'sl' | 'sr' | 'nxl' | 'nxr' | 'adj' | 'ov'

function serializeValue(v: unknown): string {
  if (v === null || v === undefined) return 'null'
  if (typeof v === 'boolean' || typeof v === 'number') return String(v)
  return String(v)
}

function escapeInListValue(v: unknown): string {
  const s = serializeValue(v)
  // Quote values containing PostgREST list delimiters.
  if (/[,\(\)":\s]/.test(s)) return `"${s.replace(/"/g, '\\"')}"`
  return s
}

export interface SelectOptions {
  count?: CountOption | null
  head?: boolean
}

export interface InsertOptions {
  count?: CountOption | null
  defaultToNull?: boolean
}

export interface UpsertOptions {
  onConflict?: string
  ignoreDuplicates?: boolean
  count?: CountOption | null
  defaultToNull?: boolean
}

export interface UpdateOptions {
  count?: CountOption | null
}

export interface DeleteOptions {
  count?: CountOption | null
}

export interface OrderOptions {
  ascending?: boolean
  nullsFirst?: boolean
  referencedTable?: string
}

export class PostgrestQueryBuilder<T = Record<string, unknown>> {
  private transport: Transport
  private table: string
  private fetchImpl?: FetchImpl
  private params = new URLSearchParams()
  private orders: string[] = []
  private action: 'select' | 'insert' | 'upsert' | 'update' | 'delete' = 'select'
  private body: unknown = undefined
  private preferHeaders: string[] = []
  private headOnly = false
  private singleRow = false
  private maybeSingleRow = false
  private limitN: number | null = null
  private offsetN: number | null = null
  private rangeFrom: number | null = null
  private rangeTo: number | null = null
  /** Structured equality filters (used to route id-addressed writes on today's backend). */
  private eqFilters: Array<{ column: string; value: unknown }> = []

  constructor(transport: Transport, table: string, fetchImpl?: FetchImpl) {
    if (!table || table.trim() === '') throw new Error('from(): table name is required')
    this.transport = transport
    this.table = table
    this.fetchImpl = fetchImpl
  }

  // ---- verbs ----

  select(columns = '*', options: SelectOptions = {}): this {
    this.action = 'select'
    this.params.set('select', columns)
    if (options.count) {
      this.preferHeaders.push(`count=${options.count}`)
    }
    if (options.head) this.headOnly = true
    return this
  }

  insert(values: Partial<T> | Array<Partial<T>>, options: InsertOptions = {}): this {
    this.action = 'insert'
    // Compat: today's backend takes a single doc object; unwrap 1-element arrays.
    if (Array.isArray(values) && values.length === 1) this.body = values[0]
    else this.body = values
    if (options.count) {
      this.preferHeaders.push(`count=${options.count}`)
    }
    this.preferHeaders.push('return=representation')
    if (options.defaultToNull === false) this.preferHeaders.push('missing=default')
    return this
  }

  upsert(values: Partial<T> | Array<Partial<T>>, options: UpsertOptions = {}): this {
    this.action = 'upsert'
    if (Array.isArray(values) && values.length === 1) this.body = values[0]
    else this.body = values
    this.preferHeaders.push('resolution=merge-duplicates', 'return=representation')
    if (options.onConflict) this.params.set('on_conflict', options.onConflict)
    if (options.ignoreDuplicates) this.params.set('ignore_duplicates', 'true')
    if (options.count) {
      this.preferHeaders.push(`count=${options.count}`)
    }
    return this
  }

  update(values: Partial<T>, options: UpdateOptions = {}): this {
    this.action = 'update'
    this.body = values
    this.preferHeaders.push('return=representation')
    if (options.count) {
      this.preferHeaders.push(`count=${options.count}`)
    }
    return this
  }

  delete(options: DeleteOptions = {}): this {
    this.action = 'delete'
    this.preferHeaders.push('return=representation')
    if (options.count) {
      this.preferHeaders.push(`count=${options.count}`)
    }
    return this
  }

  // ---- filters (every operator appends `column=op.value`) ----

  private addFilter(column: string, op: string, value: unknown): this {
    this.params.append(column, `${op}.${serializeValue(value)}`)
    return this
  }

  filter(column: string, operator: string, value: unknown): this {
    return this.addFilter(column, operator, value)
  }
  eq(column: string, value: unknown): this {
    this.eqFilters.push({ column, value })
    return this.addFilter(column, 'eq', value)
  }
  neq(column: string, value: unknown): this {
    return this.addFilter(column, 'neq', value)
  }
  gt(column: string, value: unknown): this {
    return this.addFilter(column, 'gt', value)
  }
  gte(column: string, value: unknown): this {
    return this.addFilter(column, 'gte', value)
  }
  lt(column: string, value: unknown): this {
    return this.addFilter(column, 'lt', value)
  }
  lte(column: string, value: unknown): this {
    return this.addFilter(column, 'lte', value)
  }
  like(column: string, pattern: string): this {
    return this.addFilter(column, 'like', pattern)
  }
  ilike(column: string, pattern: string): this {
    return this.addFilter(column, 'ilike', pattern)
  }
  is(column: string, value: 'null' | 'true' | 'false' | 'unknown' | null | boolean): this {
    const v = value === null ? 'null' : String(value)
    return this.addFilter(column, 'is', v)
  }
  in(column: string, values: unknown[]): this {
    const list = `(${values.map(escapeInListValue).join(',')})`
    this.params.append(column, `in.${list}`)
    return this
  }
  contains(column: string, value: unknown): this {
    const v = typeof value === 'object' ? JSON.stringify(value) : serializeValue(value)
    return this.addFilter(column, 'cs', v)
  }
  containedBy(column: string, value: unknown): this {
    const v = typeof value === 'object' ? JSON.stringify(value) : serializeValue(value)
    return this.addFilter(column, 'cd', v)
  }
  rangeGt(column: string, value: unknown): this {
    return this.addFilter(column, 'sr', value)
  }
  rangeGte(column: string, value: unknown): this {
    return this.addFilter(column, 'nxl', value)
  }
  rangeLt(column: string, value: unknown): this {
    return this.addFilter(column, 'sl', value)
  }
  rangeLte(column: string, value: unknown): this {
    return this.addFilter(column, 'nxr', value)
  }
  rangeAdjacent(column: string, value: unknown): this {
    return this.addFilter(column, 'adj', value)
  }
  overlaps(column: string, value: unknown): this {
    const v = typeof value === 'object' ? JSON.stringify(value) : serializeValue(value)
    return this.addFilter(column, 'ov', v)
  }
  textSearch(
    column: string,
    query: string,
    options: { type?: 'plain' | 'phrase' | 'websearch'; config?: string } = {},
  ): this {
    const type = options.type === 'plain' ? 'pl' : options.type === 'phrase' ? 'ph' : options.type === 'websearch' ? 'w' : 'fts'
    const op = options.config ? `${type}(${options.config})` : type
    return this.addFilter(column, op, query)
  }
  match(query: Record<string, unknown>): this {
    for (const [k, v] of Object.entries(query)) this.eq(k, v)
    return this
  }
  not(column: string, operator: string, value: unknown): this {
    this.params.append(column, `not.${operator}.${serializeValue(value)}`)
    return this
  }
  or(filters: string, referencedTable?: string): this {
    const key = referencedTable ? `${referencedTable}.or` : 'or'
    this.params.append(key, `(${filters})`)
    return this
  }
  and(filters: string, referencedTable?: string): this {
    const key = referencedTable ? `${referencedTable}.and` : 'and'
    this.params.append(key, `(${filters})`)
    return this
  }

  // ---- modifiers ----

  order(column: string, options: OrderOptions = {}): this {
    const dir = options.ascending === false ? 'desc' : 'asc'
    const nulls = options.nullsFirst === true ? '.nullsfirst' : options.nullsFirst === false ? '.nullslast' : ''
    const entry = `${column}.${dir}${nulls}`
    if (options.referencedTable) {
      this.params.append(`${options.referencedTable}.order`, entry)
    } else {
      this.orders.push(entry)
    }
    return this
  }

  limit(n: number): this {
    this.limitN = n
    return this
  }

  offset(n: number): this {
    this.offsetN = n
    return this
  }

  range(from: number, to: number): this {
    this.rangeFrom = from
    this.rangeTo = to
    return this
  }

  single(): this {
    this.singleRow = true
    this.preferHeaders.push('return=representation')
    return this
  }

  maybeSingle(): this {
    this.maybeSingleRow = true
    this.preferHeaders.push('return=representation')
    return this
  }

  csv(): this {
    // Export hint; server may honor Accept: text/csv.
    this.params.set('format', 'csv')
    return this
  }

  // ---- URL + execution ----

  /** Full request path (without base) for tests/debugging. */
  buildPath(): string {
    const pairs: Array<[string, string]> = []
    for (const [k, v] of this.params) pairs.push([k, v])
    // PostgREST `order` (full grammar, the future) + legacy `order_by`/`order`
    // compat for today's backend. Legacy `order` comes FIRST so
    // `Query.Get("order")` (first-value-wins) still sees exactly `desc|asc`,
    // while Phase 12.1 servers read the full `order` value(s).
    if (this.orders.length > 0) {
      const [first] = this.orders
      if (first && !this.params.has('order')) {
        const [col, dir] = first.split('.')
        if (col) {
          pairs.push(['order_by', col])
          pairs.push(['order', dir === 'desc' ? 'desc' : 'asc'])
        }
      }
      for (const o of this.orders) pairs.push(['order', o])
    }
    if (this.rangeFrom !== null && this.rangeTo !== null) {
      pairs.push(['offset', String(this.rangeFrom)])
      pairs.push(['limit', String(this.rangeTo - this.rangeFrom + 1)])
    } else {
      if (this.limitN !== null) pairs.push(['limit', String(this.limitN)])
      if (this.offsetN !== null) pairs.push(['offset', String(this.offsetN)])
    }
    const qs = new URLSearchParams(pairs).toString()
    return `/v1/api/${encodeURIComponent(this.table)}${qs ? `?${qs}` : ''}`
  }

  buildHeaders(): Record<string, string> {
    const h: Record<string, string> = {}
    if (this.preferHeaders.length > 0) h['Prefer'] = [...new Set(this.preferHeaders)].join(',')
    if (this.rangeFrom !== null && this.rangeTo !== null) h['Range'] = `${this.rangeFrom}-${this.rangeTo}`
    if (this.singleRow) h['Accept'] = 'application/vnd.pgrst.object+json'
    return h
  }

  /** Id-addressed fast path for today's backend: exactly one `.eq(col, id)`. */
  private idTarget(): { column: string; id: string } | null {
    if (this.eqFilters.length === 1 && this.params.toString().split('or=').length === 1) {
      const onlyEq = this.eqFilters[0]
      if (onlyEq) {
        // Ensure no other non-select/order/limit params pollute the predicate.
        const keys = [...this.params.keys()].filter((k) => k !== 'select' && k !== 'order' && k !== 'order_by' && k !== 'limit' && k !== 'offset')
        const eqKeys = keys.filter((k) => k === onlyEq.column)
        if (eqKeys.length === 1 && keys.length === 1) {
          return { column: onlyEq.column, id: serializeValue(onlyEq.value) }
        }
      }
    }
    return null
  }

  async execute(): Promise<{ data: unknown; error: import('../lib/fetch.js').Err['error'] | null; count: number | null; status: number; statusText: string }> {
    const headers = this.buildHeaders()
    switch (this.action) {
      case 'select': {
        const res = await this.transport.request<unknown>(this.buildPath(), {
          method: this.headOnly ? 'HEAD' : 'GET',
          headers,
          fetch: this.fetchImpl,
        })
        if (res.error) return { data: null, error: res.error, count: null, status: res.status, statusText: res.statusText }
        const data = normalizeRows(res.data)
        const rows = Array.isArray(data) ? data : data == null ? [] : [data]
        if (this.singleRow || this.maybeSingleRow) {
          if (rows.length === 0) {
            if (this.maybeSingleRow) return { data: null, error: null, count: res.count ?? null, status: res.status, statusText: res.statusText }
            return {
              data: null,
              error: { message: 'JSON object requested, multiple (or no) rows returned', code: 'PGRST116', details: null, hint: null, status: 406 },
              count: res.count ?? null,
              status: 406,
              statusText: res.statusText,
            }
          }
          if (rows.length > 1 && this.singleRow) {
            return {
              data: null,
              error: { message: 'JSON object requested, multiple rows returned', code: 'PGRST116', details: null, hint: null, status: 406 },
              count: res.count ?? null,
              status: 406,
              statusText: res.statusText,
            }
          }
          return { data: rows[0] ?? null, error: null, count: res.count ?? null, status: res.status, statusText: res.statusText }
        }
        return { data, error: null, count: res.count ?? null, status: res.status, statusText: res.statusText }
      }
      case 'insert':
      case 'upsert': {
        const res = await this.transport.request<unknown>(`/v1/api/${encodeURIComponent(this.table)}`, {
          method: 'POST',
          headers,
          body: this.body,
          fetch: this.fetchImpl,
        })
        if (res.error) return { data: null, error: res.error, count: null, status: res.status, statusText: res.statusText }
        return { data: normalizeRows(res.data), error: null, count: res.count ?? null, status: res.status, statusText: res.statusText }
      }
      case 'update':
      case 'delete': {
        const target = this.idTarget()
        if (target) {
          // Today's backend: PUT/DELETE /v1/api/{collection}/{id}.
          const path = `/v1/api/${encodeURIComponent(this.table)}/${encodeURIComponent(target.id)}`
          const res = await this.transport.request<unknown>(path, {
            method: this.action === 'update' ? 'PUT' : 'DELETE',
            headers,
            body: this.action === 'update' ? this.body : undefined,
            fetch: this.fetchImpl,
          })
          if (res.error) return { data: null, error: res.error, count: null, status: res.status, statusText: res.statusText }
          return { data: normalizeRows(res.data), error: null, count: res.count ?? null, status: res.status, statusText: res.statusText }
        }
        // Future grammar path (Phase 12.3): filtered PATCH/DELETE.
        const res = await this.transport.request<unknown>(this.buildPath(), {
          method: this.action === 'update' ? 'PATCH' : 'DELETE',
          headers,
          body: this.action === 'update' ? this.body : undefined,
          fetch: this.fetchImpl,
        })
        if (res.error) return { data: null, error: res.error, count: null, status: res.status, statusText: res.statusText }
        return { data: normalizeRows(res.data), error: null, count: res.count ?? null, status: res.status, statusText: res.statusText }
      }
    }
  }

  // Thenable: `await openbase.from('t').select()` works directly.
  then<TResult1 = unknown, TResult2 = never>(
    onfulfilled?: ((value: { data: unknown; error: import('../lib/fetch.js').Err['error'] | null; count: number | null; status: number; statusText: string }) => TResult1 | Promise<TResult1>) | null,
    onrejected?: ((reason: unknown) => TResult2 | Promise<TResult2>) | null,
  ): Promise<TResult1 | TResult2> {
    return this.execute().then(onfulfilled, onrejected)
  }
}

/** Normalize backend shapes: array passthrough, `{ rows }` → rows, else as-is. */
export function normalizeRows(input: unknown): unknown {
  if (Array.isArray(input)) return input
  if (input && typeof input === 'object') {
    const o = input as Record<string, unknown>
    if (Array.isArray(o['rows'])) return o['rows']
    if (Array.isArray(o['data'])) return o['data']
  }
  return input
}

export class PostgrestClient {
  constructor(
    private transport: Transport,
    private fetchImpl?: FetchImpl,
  ) {}

  from<T = Record<string, unknown>>(table: string): PostgrestQueryBuilder<T> {
    return new PostgrestQueryBuilder<T>(this.transport, table, this.fetchImpl)
  }

  /** Call a database function (Phase 12.4; capability-gated server-side). */
  async rpc<T = unknown>(
    fn: string,
    args: Record<string, unknown> = {},
    options: { count?: CountOption | null; head?: boolean } = {},
  ): Promise<{ data: T | null; error: import('../lib/fetch.js').Err['error'] | null; count: number | null; status: number }> {
    if (!fn || fn.trim() === '') {
      return { data: null, error: { message: 'rpc(): function name is required', code: 'validation', details: null, hint: null }, count: null, status: 400 }
    }
    const headers: Record<string, string> = {}
    if (options.count) headers['Prefer'] = `count=${options.count}`
    const res = await this.transport.request<T>(`/v1/api/rpc/${encodeURIComponent(fn)}`, {
      method: 'POST',
      headers,
      body: args,
      fetch: this.fetchImpl,
    })
    if (res.error) return { data: null, error: res.error, count: null, status: res.status }
    return { data: res.data, error: null, count: res.count ?? null, status: res.status }
  }
}
