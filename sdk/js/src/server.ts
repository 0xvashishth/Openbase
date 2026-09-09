// Server entry (`@openbase/js/server`): Next.js App Router / SSR helpers.
// Cookie-backed session storage + thin createServerClient/createBrowserClient
// wrappers mirroring `@supabase/ssr`.

import { OpenbaseClient, type OpenbaseClientOptions } from './client.js'
import { cookieStorageAdapter } from './lib/storage.js'
import type { FetchImpl } from './lib/fetch.js'

export interface CookieStore {
  get(name: string): string | undefined
  set(name: string, value: string, options?: Record<string, unknown>): void
  remove(name: string): void
}

export interface ServerClientOptions extends Omit<OpenbaseClientOptions, 'auth' | 'global'> {
  cookies: CookieStore
  auth?: Omit<NonNullable<OpenbaseClientOptions['auth']>, 'storage'>
  global?: Omit<NonNullable<OpenbaseClientOptions['global']>, 'fetch'> & { fetch?: FetchImpl }
}

/** SSR client: sessions persist in cookies so Server Components see the user. */
export function createServerClient(url: string, key: string, options: ServerClientOptions): OpenbaseClient {
  return new OpenbaseClient(url, key, {
    ...options,
    auth: {
      ...options.auth,
      storage: cookieStorageAdapter(options.cookies),
      // SSR renders must not run timers or touch window.location.
      autoRefreshToken: false,
      detectSessionInUrl: false,
      persistSession: true,
    },
    global: options.global,
  } as OpenbaseClientOptions)
}

/** Browser client: localStorage persistence + URL detection (PKCE/implicit). */
export function createBrowserClient(url: string, key: string, options: OpenbaseClientOptions = {}): OpenbaseClient {
  return new OpenbaseClient(url, key, {
    ...options,
    auth: { persistSession: true, autoRefreshToken: true, detectSessionInUrl: true, ...options.auth },
  })
}

export { cookieStorageAdapter }
export { createClient, OpenbaseClient } from './client.js'
