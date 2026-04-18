// Smackerel — CaptureQueue for PWA (IndexedDB)
// Shared offline queue logic — same as extension/lib/queue.js
'use strict';

var CaptureQueue = (function() {
  var DB_NAME = 'smackerel-queue';
  var DB_VERSION = 1;
  var STORE_NAME = 'pending';
  var MAX_ITEMS = 100;

  function open() {
    return new Promise(function(resolve, reject) {
      var request = indexedDB.open(DB_NAME, DB_VERSION);
      request.onupgradeneeded = function(event) {
        var db = event.target.result;
        if (!db.objectStoreNames.contains(STORE_NAME)) {
          db.createObjectStore(STORE_NAME, { keyPath: 'id', autoIncrement: true });
        }
      };
      request.onsuccess = function(event) { resolve(event.target.result); };
      request.onerror = function(event) { reject(event.target.error); };
    });
  }

  function enqueue(item) {
    return open().then(function(db) {
      return new Promise(function(resolve, reject) {
        var tx = db.transaction(STORE_NAME, 'readwrite');
        var store = tx.objectStore(STORE_NAME);

        var countReq = store.count();
        countReq.onsuccess = function() {
          if (countReq.result >= MAX_ITEMS) {
            resolve(false);
            return;
          }
          store.add({
            url: item.url || '',
            title: item.title || '',
            text: item.text || '',
            capturedAt: new Date().toISOString(),
            status: 'pending'
          });
          tx.oncomplete = function() { resolve(true); };
          tx.onerror = function() { reject(tx.error); };
        };
      });
    });
  }

  function flush(apiUrl, authToken) {
    return open().then(function(db) {
      return new Promise(function(resolve) {
        var tx = db.transaction(STORE_NAME, 'readonly');
        var store = tx.objectStore(STORE_NAME);
        var getAll = store.getAll();

        getAll.onsuccess = function() {
          var items = getAll.result;
          if (items.length === 0) {
            resolve({ flushed: 0, errors: 0, authFailed: false });
            return;
          }

          var flushed = 0;
          var errors = 0;
          var authFailed = false;
          var deleteIds = [];

          var chain = Promise.resolve();
          items.forEach(function(item) {
            chain = chain.then(function() {
              if (authFailed) return;

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
                  flushed++;
                  deleteIds.push(item.id);
                } else if (resp.status === 401) {
                  authFailed = true;
                } else {
                  errors++;
                }
              })
              .catch(function() {
                errors++;
              });
            });
          });

          chain.then(function() {
            if (deleteIds.length > 0) {
              var delTx = db.transaction(STORE_NAME, 'readwrite');
              var delStore = delTx.objectStore(STORE_NAME);
              deleteIds.forEach(function(id) { delStore.delete(id); });
            }
            resolve({ flushed: flushed, errors: errors, authFailed: authFailed });
          });
        };
      });
    });
  }

  function count() {
    return open().then(function(db) {
      return new Promise(function(resolve) {
        var tx = db.transaction(STORE_NAME, 'readonly');
        var req = tx.objectStore(STORE_NAME).count();
        req.onsuccess = function() { resolve(req.result); };
        req.onerror = function() { resolve(0); };
      });
    });
  }

  function clear() {
    return open().then(function(db) {
      return new Promise(function(resolve, reject) {
        var tx = db.transaction(STORE_NAME, 'readwrite');
        tx.objectStore(STORE_NAME).clear();
        tx.oncomplete = function() { resolve(); };
        tx.onerror = function() { reject(tx.error); };
      });
    });
  }

  return {
    enqueue: enqueue,
    flush: flush,
    count: count,
    clear: clear
  };
})();
