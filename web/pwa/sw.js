// Smackerel PWA Service Worker
var CACHE_NAME = 'smackerel-pwa-v1';
var STATIC_ASSETS = [
  '/pwa/',
  '/pwa/index.html',
  '/pwa/style.css',
  '/pwa/icon.svg',
  '/pwa/manifest.json',
  '/pwa/lib/queue.js'
];

// Install: cache static assets
self.addEventListener('install', function(event) {
  event.waitUntil(
    caches.open(CACHE_NAME).then(function(cache) {
      return cache.addAll(STATIC_ASSETS);
    })
  );
  self.skipWaiting();
});

// Activate: clean old caches
self.addEventListener('activate', function(event) {
  event.waitUntil(
    caches.keys().then(function(names) {
      return Promise.all(
        names.filter(function(name) { return name !== CACHE_NAME; })
             .map(function(name) { return caches.delete(name); })
      );
    })
  );
  self.clients.claim();
});

// Fetch: cache-first for static assets, network-first for API calls
self.addEventListener('fetch', function(event) {
  var url = new URL(event.request.url);

  // Don't cache API calls or POST requests
  if (url.pathname.startsWith('/api/') || event.request.method !== 'GET') {
    return;
  }

  // Cache-first for PWA static assets
  if (url.pathname.startsWith('/pwa/')) {
    event.respondWith(
      caches.match(event.request).then(function(cached) {
        return cached || fetch(event.request).then(function(response) {
          if (response.ok) {
            var clone = response.clone();
            caches.open(CACHE_NAME).then(function(cache) {
              cache.put(event.request, clone);
            });
          }
          return response;
        });
      })
    );
  }
});

// Background Sync: flush offline queue when connectivity restores (Scope 5)
self.addEventListener('sync', function(event) {
  if (event.tag === 'smackerel-sync') {
    event.waitUntil(flushOfflineQueue());
  }
});

// Periodic sync fallback — also try flushing on any fetch success
function flushOfflineQueue() {
  // Read config from the client's localStorage via message
  return self.clients.matchAll().then(function(clients) {
    if (clients.length === 0) return;
    // Ask first client for config
    return new Promise(function(resolve) {
      var mc = new MessageChannel();
      mc.port1.onmessage = function(event) {
        var config = event.data;
        if (config && config.serverUrl && config.authToken) {
          flushWithConfig(config.serverUrl, config.authToken).then(resolve);
        } else {
          resolve();
        }
      };
      clients[0].postMessage({ type: 'getConfig' }, [mc.port2]);
    });
  });
}

function flushWithConfig(serverUrl, authToken) {
  var DB_NAME = 'smackerel-queue';
  var STORE_NAME = 'pending';

  return new Promise(function(resolve) {
    var request = indexedDB.open(DB_NAME, 1);
    request.onsuccess = function(event) {
      var db = event.target.result;
      if (!db.objectStoreNames.contains(STORE_NAME)) { resolve(); return; }

      var tx = db.transaction(STORE_NAME, 'readonly');
      var getAll = tx.objectStore(STORE_NAME).getAll();
      getAll.onsuccess = function() {
        var items = getAll.result;
        if (items.length === 0) { resolve(); return; }

        var apiUrl = serverUrl.replace(/\/+$/, '') + '/api/capture';
        var deleteIds = [];
        var chain = Promise.resolve();

        items.forEach(function(item) {
          chain = chain.then(function() {
            var body = {};
            if (item.url) body.url = item.url;
            if (item.text) body.text = item.text;
            if (item.title) body.context = 'Captured (offline): ' + item.title;

            return fetch(apiUrl, {
              method: 'POST',
              headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + authToken
              },
              body: JSON.stringify(body)
            })
            .then(function(resp) {
              if (resp.ok || resp.status === 409) {
                deleteIds.push(item.id);
              }
            })
            .catch(function() { /* keep in queue */ });
          });
        });

        chain.then(function() {
          if (deleteIds.length > 0) {
            var delTx = db.transaction(STORE_NAME, 'readwrite');
            var delStore = delTx.objectStore(STORE_NAME);
            deleteIds.forEach(function(id) { delStore.delete(id); });
          }
          resolve();
        });
      };
    };
    request.onerror = function() { resolve(); };
  });
}
