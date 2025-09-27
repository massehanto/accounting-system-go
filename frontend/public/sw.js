const CACHE_NAME = 'accounting-system-v1';
const STATIC_CACHE_NAME = 'accounting-static-v1';
const DYNAMIC_CACHE_NAME = 'accounting-dynamic-v1';

// Assets to cache on install
const STATIC_ASSETS = [
  '/',
  '/index.html',
  '/manifest.json',
  '/favicon.ico',
  '/static/css/main.css',
  '/static/js/bundle.js'
];

// Install event - cache static assets
self.addEventListener('install', (event) => {
  console.log('Service Worker installing...');
  
  event.waitUntil(
    caches.open(STATIC_CACHE_NAME)
      .then(cache => {
        console.log('Caching static assets');
        return cache.addAll(STATIC_ASSETS);
      })
      .then(() => self.skipWaiting())
  );
});

// Activate event - clean up old caches
self.addEventListener('activate', (event) => {
  console.log('Service Worker activating...');
  
  event.waitUntil(
    caches.keys()
      .then(cacheNames => {
        return Promise.all(
          cacheNames
            .filter(cacheName => {
              return cacheName.startsWith('accounting-') && 
                     cacheName !== STATIC_CACHE_NAME &&
                     cacheName !== DYNAMIC_CACHE_NAME;
            })
            .map(cacheName => caches.delete(cacheName))
        );
      })
      .then(() => self.clients.claim())
  );
});

// Fetch event - network first, fallback to cache
self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // Skip chrome-extension and non-http(s) requests
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    return;
  }

  // API calls - network first with cache fallback
  if (url.pathname.startsWith('/api')) {
    event.respondWith(
      fetch(request)
        .then(response => {
          // Clone the response before caching
          const responseToCache = response.clone();
          
          // Only cache successful GET requests
          if (request.method === 'GET' && response.status === 200) {
            caches.open(DYNAMIC_CACHE_NAME)
              .then(cache => cache.put(request, responseToCache));
          }
          
          return response;
        })
        .catch(() => {
          // If network fails, try cache
          return caches.match(request)
            .then(response => {
              if (response) {
                // Add header to indicate cached response
                const headers = new Headers(response.headers);
                headers.set('X-From-Cache', 'true');
                
                return new Response(response.body, {
                  status: response.status,
                  statusText: response.statusText,
                  headers: headers
                });
              }
              
              // Return offline fallback for GET requests
              if (request.method === 'GET') {
                return new Response(
                  JSON.stringify({
                    error: 'You are offline',
                    code: 'OFFLINE',
                    cached: false
                  }),
                  {
                    status: 503,
                    headers: { 'Content-Type': 'application/json' }
                  }
                );
              }
              
              throw new Error('Offline');
            });
        })
    );
    return;
  }

  // Static assets - cache first
  event.respondWith(
    caches.match(request)
      .then(response => {
        if (response) {
          return response;
        }

        return fetch(request)
          .then(response => {
            // Don't cache non-successful responses
            if (!response || response.status !== 200 || response.type !== 'basic') {
              return response;
            }

            const responseToCache = response.clone();

            caches.open(STATIC_CACHE_NAME)
              .then(cache => cache.put(request, responseToCache));

            return response;
          });
      })
      .catch(() => {
        // Offline fallback page
        if (request.destination === 'document') {
          return caches.match('/offline.html');
        }
      })
  );
});

// Background sync for offline requests
self.addEventListener('sync', (event) => {
  if (event.tag === 'sync-requests') {
    event.waitUntil(syncOfflineRequests());
  }
});

async function syncOfflineRequests() {
  const cache = await caches.open('offline-requests');
  const requests = await cache.keys();
  
  for (const request of requests) {
    try {
      const response = await fetch(request);
      if (response.ok) {
        await cache.delete(request);
        
        // Notify client of successful sync
        const clients = await self.clients.matchAll();
        clients.forEach(client => {
          client.postMessage({
            type: 'SYNC_SUCCESS',
            url: request.url
          });
        });
      }
    } catch (error) {
      console.error('Sync failed for', request.url, error);
    }
  }
}

// Push notifications
self.addEventListener('push', (event) => {
  if (event.data) {
    const data = event.data.json();
    
    const options = {
      body: data.body || 'New notification',
      icon: '/icon-192x192.png',
      badge: '/badge-72x72.png',
      vibrate: [100, 50, 100],
      data: {
        url: data.url || '/',
        dateOfArrival: Date.now()
      },
      actions: [
        {
          action: 'view',
          title: 'View',
          icon: '/icons/checkmark.png'
        },
        {
          action: 'close',
          title: 'Close',
          icon: '/icons/xmark.png'
        }
      ]
    };
    
    event.waitUntil(
      self.registration.showNotification(data.title || 'Accounting System', options)
    );
  }
});

self.addEventListener('notificationclick', (event) => {
  const notification = event.notification;
  const action = event.action;
  
  notification.close();
  
  if (action === 'close') {
    return;
  }
  
  event.waitUntil(
    clients.openWindow(notification.data.url)
  );
});

// Periodic background sync
self.addEventListener('periodicsync', (event) => {
  if (event.tag === 'update-reports') {
    event.waitUntil(updateReportsInBackground());
  }
});

async function updateReportsInBackground() {
  try {
    const response = await fetch('/api/reports/summary');
    const data = await response.json();
    
    // Cache the updated data
    const cache = await caches.open(DYNAMIC_CACHE_NAME);
    await cache.put('/api/reports/summary', new Response(JSON.stringify(data)));
    
    // Notify clients
    const clients = await self.clients.matchAll();
    clients.forEach(client => {
      client.postMessage({
        type: 'REPORTS_UPDATED',
        data: data
      });
    });
  } catch (error) {
    console.error('Background report update failed:', error);
  }
}