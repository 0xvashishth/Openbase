// Package entry: createClient + full surface re-exports.

export { createClient, OpenbaseClient, type OpenbaseClientOptions } from './client.js'
export { AuthClient } from './auth/client.js'
export { PostgrestClient, PostgrestQueryBuilder, normalizeRows } from './postgrest/builder.js'
export { RealtimeClient, RealtimeChannel, matchesRowFilter } from './realtime/channel.js'
export { StorageClient, StorageFileApi } from './storage/client.js'
export { FunctionsClient } from './functions/client.js'
export { OpenbaseError, CapabilityError, toPostgrestError } from './lib/errors.js'
export { MemoryStorage, LocalStorageAdapter, defaultStorage, cookieStorageAdapter } from './lib/storage.js'
export type { Session, User, AuthChangeEvent, AuthChangeCallback, CapabilitySet } from './lib/types.js'
