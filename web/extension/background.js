// Smackerel Browser Extension — Background Service Worker
// Handles: context menu capture, toolbar capture, offline queue, notifications
'use strict';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getConfig() {
  return new Promise(function(resolve) {
    chrome.storage.local.get(['serverUrl', 'authToken'], function(data) {
      resolve({ serverUrl: data.serverUrl || '', authToken: data.authToken || '' });
    });
  });
}

// ---------------------------------------------------------------------------
// Context Menu Setup (Scope 4)
// ---------------------------------------------------------------------------

chrome.runtime.onInstalled.addListener(function() {
  chrome.contextMenus.create({
    id: 'smackerel-save-page',
    title: 'Save to Smackerel',
    contexts: ['page', 'link', 'image']
  });

  chrome.contextMenus.create({
    id: 'smackerel-save-selection',
    title: 'Save with selection',
    contexts: ['selection']
  });
});

// ---------------------------------------------------------------------------
// Context Menu Click Handler (Scope 4)
// ---------------------------------------------------------------------------

chrome.contextMenus.onClicked.addListener(function(info, tab) {
  var captureData = {
    url: info.linkUrl || info.pageUrl || (tab ? tab.url : ''),
    title: tab ? tab.title : '',
    text: ''
  };

  if (info.menuItemId === 'smackerel-save-selection' && info.selectionText) {
    captureData.text = info.selectionText;
  }

  doCapture(captureData);
});

// ---------------------------------------------------------------------------
// Toolbar Button Click (Scope 4)
// ---------------------------------------------------------------------------

// When user clicks the toolbar icon without opening popup (if popup is closed quickly),
// this fires. However with default_popup set, the popup opens instead.
// Capture is triggered from popup.js via message passing.

// ---------------------------------------------------------------------------
// Message Handler — popup.js communicates via messages
// ---------------------------------------------------------------------------

chrome.runtime.onMessage.addListener(function(msg, sender, sendResponse) {
  if (msg.action === 'capture') {
    doCapture(msg.data).then(function(result) {
      sendResponse(result);
    });
    return true; // async sendResponse
  }

  if (msg.action === 'getQueueCount') {
    getQueueCount().then(function(count) {
      sendResponse({ count: count });
    });
    return true;
  }

  if (msg.action === 'flushQueue') {
    flushQueue().then(function(result) {
      sendResponse(result);
    });
    return true;
  }
});

// ---------------------------------------------------------------------------
// Flush Serialization Guard (chaos-hardening: prevent concurrent flushes)
// ---------------------------------------------------------------------------

var flushInProgress = false;

// ---------------------------------------------------------------------------
// Capture Logic
// ---------------------------------------------------------------------------

function doCapture(data) {
  return getConfig().then(function(config) {
    if (!config.serverUrl || !config.authToken) {
      showNotification('Setup Required', 'Please configure Smackerel in the extension popup.');
      return { success: false, error: 'not_configured' };
    }

    var body = {};
    if (data.url) body.url = data.url;
    if (data.text) body.text = data.text;
    if (data.title) body.context = 'Captured: ' + data.title;

    if (!body.url && !body.text) {
      showNotification('Nothing to Capture', 'No URL or text to save.');
      return { success: false, error: 'empty_capture' };
    }

    var apiUrl = config.serverUrl.replace(/\/+$/, '') + '/api/capture';

    return fetch(apiUrl, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer ' + config.authToken,
        'X-Capture-Source': 'extension'
      },
      body: JSON.stringify(body)
    })
    .then(function(resp) {
      if (resp.ok || resp.status === 409) {
        showNotification('Saved!', data.title || data.url || 'Content captured');
        return { success: true };
      }
      if (resp.status === 401) {
        showNotification('Auth Failed', 'Please check your auth token in settings.');
        return { success: false, error: 'auth_failed' };
      }
      return resp.text().then(function(text) {
        showNotification('Save Failed', 'Server returned ' + resp.status);
        return { success: false, error: 'http_' + resp.status };
      });
    })
    .catch(function(err) {
      // Network error — queue offline (Scope 5)
      return enqueueOffline(data).then(function() {
        showNotification('Saved Offline', 'Will sync when connected');
        return { success: true, queued: true };
      }).catch(function(queueErr) {
        if (queueErr && queueErr.message === 'queue_full') {
          showNotification('Queue Full', 'Offline queue is full — please sync pending items first');
          return { success: false, error: 'queue_full' };
        }
        showNotification('Save Failed', 'Could not save offline');
        return { success: false, error: 'queue_error' };
      });
    });
  });
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

function showNotification(title, message) {
  chrome.notifications.create({
    type: 'basic',
    iconUrl: 'icons/icon-128.svg',
    title: title,
    message: message
  });
}

// ---------------------------------------------------------------------------
// Offline Queue — IndexedDB (Scope 5)
// ---------------------------------------------------------------------------

var DB_NAME = 'smackerel-queue';
var DB_VERSION = 1;
var STORE_NAME = 'pending';
var MAX_QUEUE_SIZE = 100;

function openDB() {
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

function enqueueOffline(data) {
  return openDB().then(function(db) {
    return new Promise(function(resolve, reject) {
      var tx = db.transaction(STORE_NAME, 'readwrite');
      var store = tx.objectStore(STORE_NAME);

      // Check count before adding
      var countReq = store.count();
      countReq.onsuccess = function() {
        if (countReq.result >= MAX_QUEUE_SIZE) {
          reject(new Error('queue_full'));
          return;
        }
        store.add({
          url: data.url || '',
          title: data.title || '',
          text: data.text || '',
          capturedAt: new Date().toISOString(),
          status: 'pending'
        });
        tx.oncomplete = function() { resolve(); };
        tx.onerror = function() { reject(tx.error); };
      };
    });
  });
}

function getQueueCount() {
  return openDB().then(function(db) {
    return new Promise(function(resolve) {
      var tx = db.transaction(STORE_NAME, 'readonly');
      var req = tx.objectStore(STORE_NAME).count();
      req.onsuccess = function() { resolve(req.result); };
      req.onerror = function() { resolve(0); };
    });
  });
}

function flushQueue() {
  if (flushInProgress) {
    return Promise.resolve({ flushed: 0, errors: 0, authFailed: false, skipped: true });
  }
  flushInProgress = true;

  return getConfig().then(function(config) {
    if (!config.serverUrl || !config.authToken) {
      return { flushed: 0, errors: 0, authFailed: false };
    }

    return openDB().then(function(db) {
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

          var apiUrl = config.serverUrl.replace(/\/+$/, '') + '/api/capture';
          var flushed = 0;
          var errors = 0;
          var authFailed = false;
          var deleteIds = [];

          var chain = Promise.resolve();
          items.forEach(function(item) {
            chain = chain.then(function() {
              if (authFailed) return; // stop on auth failure

              var body = {};
              if (item.url) body.url = item.url;
              if (item.text) body.text = item.text;
              if (item.title) body.context = 'Captured (offline): ' + item.title;

              return fetch(apiUrl, {
                method: 'POST',
                headers: {
                  'Content-Type': 'application/json',
                  'Authorization': 'Bearer ' + config.authToken,
                  'X-Capture-Source': 'extension'
                },
                body: JSON.stringify(body)
              })
              .then(function(resp) {
                if (resp.ok || resp.status === 409) {
                  flushed++;
                  deleteIds.push(item.id);
                } else if (resp.status === 401) {
                  authFailed = true;
                  // Preserve all remaining items on auth failure
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
            // Delete successfully flushed items
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
  }).finally(function() {
    flushInProgress = false;
  });
}

// ---------------------------------------------------------------------------
// Connectivity Listener — auto-flush queue when back online (Scope 5)
// ---------------------------------------------------------------------------

// Service workers don't have window.addEventListener('online'),
// but we can use chrome.alarms or periodic checks.
// Use a simple alarm-based approach.

chrome.alarms.create('smackerel-sync', { periodInMinutes: 1 });

chrome.alarms.onAlarm.addListener(function(alarm) {
  if (alarm.name === 'smackerel-sync') {
    // Try to flush — if offline, requests will fail silently
    getQueueCount().then(function(count) {
      if (count > 0) {
        flushQueue().then(function(result) {
          if (result.flushed > 0) {
            showNotification('Queue Synced', result.flushed + ' item(s) saved');
          }
          if (result.authFailed) {
            showNotification('Auth Expired', 'Please update your auth token');
          }
        });
      }
    });
  }
});
