/**
 * Openbase Dashboard Service Worker
 * Stale-while-revalidate for assets, network-first for API
 * Version: increment on each deploy to bust caches
 */
const CACHE_VERSION = 'v1';
const STATIC_CACHE = `openbase-static-${CACHE_VERSION}`;
const API_CACHE = `openbase-api-${CACHE_VERSION}`;
const OFFLINE_CACHE = `openbase-offline-${CACHE_VERSION}`;

// Assets to precache
const PRECACHE_ASSETS = [
  '/',
  '/orgs',
  '/projects',
  '/manifest.json',
  '/icons/icon-192.png',
  '/icons/icon-512.png',
];

// Cache strategies
const STRATEGIES = {
  // Static assets: stale-while-revalidate
  static: async (request, cache) => {
    const cached = await cache.match(request);
    const fetchPromise = fetch(request).then((response) => {
      if (response.ok) cache.put(request, response.clone());
      return response;
    }).catch(() => cached);
    return cached || fetchPromise;
  },

  // API calls: network-first with 5min cache
  api: async (request, cache) => {
    try {
      const response = await fetch(request);
      if (response.ok) {
        const cloned = response.clone();
        // Add timestamp for TTL
        const headers = new Headers(cloned.headers);
        headers.set('sw-cached-at', Date.now().toString());
        const cachedResponse = new Response(cloned.body, {
          status: cloned.status,
          statusText: cloned.statusText,
          headers,
        });
        cache.put(request, cachedResponse);
      }
      return response;
    } catch {
      const cached = await cache.match(request);
      if (cached) {
        const cachedAt = cached.headers.get('sw-cached-at');
        const age = cachedAt ? Date.now() - parseInt(cachedAt, 10) : Infinity;
        // 5 minute TTL
        if (age < 5 * 60 * 1000) return cached;
      }
      // Return offline fallback for navigation requests
      if (request.mode === 'navigate') {
        return caches.match('/orgs');
      }
      throw new Error('Offline');
    }
  },

  // Navigation: network-first, fallback to cached shell
  navigate: async (request, cache) => {
    try {
      const response = await fetch(request);
      if (response.ok) cache.put(request, response.clone());
      return response;
    } catch {
      const cached = await cache.match(request);
      return cached || caches.match('/orgs');
    }
  },
};

// Install: precache static assets
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(STATIC_CACHE).then((cache) => cache.addAll(PRECACHE_ASSETS))
  );
  self.skipWaiting();
});

// Activate: clean old caches
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(
        keys
          .filter((key) => ![STATIC_CACHE, API_CACHE, OFFLINE_CACHE].includes(key))
          .map((key) => caches.delete(key))
      )
    )
  );
  self.clients.claim();
});

// Fetch: route to appropriate strategy
self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // Skip non-GET and non-HTTP(S)
  if (request.method !== 'GET' || !url.protocol.startsWith('http')) return;

  // API routes: network-first with cache
  if (url.pathname.startsWith('/v1/') || url.pathname.startsWith('/api/')) {
    event.respondWith(STRATEGIES.api(request, caches.open(API_CACHE)));
    return;
  }

  // Navigation requests
  if (request.mode === 'navigate') {
    event.respondWith(STRATEGIES.navigate(request, caches.open(STATIC_CACHE)));
    return;
  }

  // Static assets: stale-while-revalidate
  if (
    url.pathname.startsWith('/_next/static/') ||
    url.pathname.startsWith('/icons/') ||
    url.pathname.endsWith('.png') ||
    url.pathname.endsWith('.ico') ||
    url.pathname.endsWith('.woff2') ||
    url.pathname === '/manifest.json'
  ) {
    event.respondWith(STRATEGIES.static(request, caches.open(STATIC_CACHE)));
    return;
  }

  // Default: network-first
  event.respondWith(
    fetch(request)
      .then((response) => {
        if (response.ok) {
          const cache = caches.open(STATIC_CACHE);
          cache.then((c) => c.put(request, response.clone()));
        }
        return response;
      })
      .catch(() => caches.match(request))
  );
});

// Background sync for offline mutations (future)
self.addEventListener('sync', (event) => {
  if (event.tag === 'sync-mutations') {
    event.waitUntil(syncMutations());
  }
});

async function syncMutations() {
  // Placeholder for future offline mutation queue
  console.log('[SW] Background sync triggered');
}

// Push notifications (future)
self.addEventListener('push', (event) => {
  if (!event.data) return;
  const data = event.data.json();
  event.waitUntil(
    self.registration.showNotification(data.title, {
      body: data.body,
      icon: '/icons/icon-192.png',
      badge: '/icons/icon-72.png',
      data: data.url,
    })
  );
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  event.waitUntil(
    clients.matchAll({ type: 'window' }).then((windowClients) => {
      for (const client of windowClients) {
        if (client.url === event.notification.data && 'focus' in client) {
          return client.focus();
        }
      }
      return clients.openWindow(event.notification.data || '/orgs');
    })
  );
});