// Session storage adapters. The auth client persists `{ session }` under a
// namespaced key. localStorage is the web default; memory is the Node default;
// any object implementing get/set/remove works (cookies, AsyncStorage, …).

export interface SessionStorageAdapter {
  getItem(key: string): string | null | Promise<string | null>
  setItem(key: string, value: string): void | Promise<void>
  removeItem(key: string): void | Promise<void>
}

export class MemoryStorage implements SessionStorageAdapter {
  private store = new Map<string, string>()
  getItem(key: string): string | null {
    return this.store.has(key) ? (this.store.get(key) as string) : null
  }
  setItem(key: string, value: string): void {
    this.store.set(key, value)
  }
  removeItem(key: string): void {
    this.store.delete(key)
  }
  clear(): void {
    this.store.clear()
  }
}

function hasLocalStorage(): boolean {
  try {
    return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined'
  } catch {
    return false
  }
}

export class LocalStorageAdapter implements SessionStorageAdapter {
  getItem(key: string): string | null {
    if (!hasLocalStorage()) return null
    return window.localStorage.getItem(key)
  }
  setItem(key: string, value: string): void {
    if (!hasLocalStorage()) return
    window.localStorage.setItem(key, value)
  }
  removeItem(key: string): void {
    if (!hasLocalStorage()) return
    window.localStorage.removeItem(key)
  }
}

/** Default: localStorage on web, in-memory elsewhere. */
export function defaultStorage(): SessionStorageAdapter {
  return hasLocalStorage() ? new LocalStorageAdapter() : new MemoryStorage()
}

/** Cookie-backed adapter interface for SSR (see src/server.ts for wiring). */
export function cookieStorageAdapter(cookies: {
  get(name: string): string | undefined
  set(name: string, value: string, options?: Record<string, unknown>): void
  remove(name: string): void
}): SessionStorageAdapter {
  return {
    getItem: (key: string) => cookies.get(key) ?? null,
    setItem: (key: string, value: string) => cookies.set(key, value, { path: '/', httpOnly: false, sameSite: 'lax' }),
    removeItem: (key: string) => cookies.remove(key),
  }
}
