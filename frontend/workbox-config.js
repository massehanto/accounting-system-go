module.exports = {
  globDirectory: 'build/',
  globPatterns: [
    '**/*.{js,css,html,png,jpg,jpeg,gif,svg,woff,woff2,ttf,eot,ico,webp,json}'
  ],
  swDest: 'build/sw.js',
  clientsClaim: true,
  skipWaiting: true,
  
  // Runtime caching strategies
  runtimeCaching: [
    // API calls - Network first, fallback to cache
    {
      urlPattern: /^https?:\/\/localhost:8000\/api\/.*/,
      handler: 'NetworkFirst',
      options: {
        cacheName: 'api-cache',
        networkTimeoutSeconds: 5,
        expiration: {
          maxEntries: 50,
          maxAgeSeconds: 300 // 5 minutes
        },
        cacheableResponse: {
          statuses: [0, 200]
        }
      }
    },
    // Images - Cache first
    {
      urlPattern: /\.(?:png|gif|jpg|jpeg|svg|webp)$/,
      handler: 'CacheFirst',
      options: {
        cacheName: 'image-cache',
        expiration: {
          maxEntries: 100,
          maxAgeSeconds: 30 * 24 * 60 * 60 // 30 days
        }
      }
    },
    // Fonts - Cache first
    {
      urlPattern: /\.(?:woff|woff2|ttf|eot)$/,
      handler: 'CacheFirst',
      options: {
        cacheName: 'font-cache',
        expiration: {
          maxEntries: 10,
          maxAgeSeconds: 365 * 24 * 60 * 60 // 1 year
        }
      }
    }
  ],
  
  // Offline fallback
  offlinePage: '/offline.html',
  
  // Background sync
  backgroundSync: {
    name: 'api-queue',
    options: {
      maxRetentionTime: 24 * 60 // Retry for up to 24 hours
    }
  }
};