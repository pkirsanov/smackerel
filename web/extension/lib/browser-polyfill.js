// Smackerel — Browser Namespace Polyfill
// Provides a unified `browser` API that works across Chrome and Firefox.
// Chrome uses `chrome.*` APIs; Firefox uses `browser.*` with Promises.
// This polyfill makes Chrome's callback-based APIs available under `browser.*`.
'use strict';

if (typeof globalThis.browser === 'undefined') {
  globalThis.browser = (function() {

    function wrapAsync(fn) {
      return function() {
        var args = Array.prototype.slice.call(arguments);
        return new Promise(function(resolve, reject) {
          args.push(function(result) {
            if (chrome.runtime.lastError) {
              reject(new Error(chrome.runtime.lastError.message));
            } else {
              resolve(result);
            }
          });
          fn.apply(null, args);
        });
      };
    }

    // Wrap commonly used Chrome APIs
    var wrapped = {
      storage: {
        local: {
          get: wrapAsync(chrome.storage.local.get.bind(chrome.storage.local)),
          set: wrapAsync(chrome.storage.local.set.bind(chrome.storage.local)),
          remove: wrapAsync(chrome.storage.local.remove.bind(chrome.storage.local))
        }
      },
      tabs: {
        query: wrapAsync(chrome.tabs.query.bind(chrome.tabs)),
        get: wrapAsync(chrome.tabs.get.bind(chrome.tabs))
      },
      runtime: {
        sendMessage: wrapAsync(chrome.runtime.sendMessage.bind(chrome.runtime)),
        getURL: chrome.runtime.getURL.bind(chrome.runtime),
        onMessage: chrome.runtime.onMessage,
        onInstalled: chrome.runtime.onInstalled,
        lastError: chrome.runtime.lastError
      },
      notifications: {
        create: wrapAsync(chrome.notifications.create.bind(chrome.notifications))
      },
      contextMenus: chrome.contextMenus,
      alarms: chrome.alarms
    };

    return wrapped;
  })();
}
