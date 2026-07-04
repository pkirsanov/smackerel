// Smackerel PWA — App Shell Script
// Handles service worker registration, background sync, and the install prompt.
//
// Spec 100 SCOPE-03: the PWA authenticates via the same-origin HttpOnly
// auth_token cookie (set by /login). There is NO pasted bearer token and NO
// server-URL / auth-token localStorage configuration — every capture/share
// fetch is same-origin with credentials, so the browser attaches the cookie.
'use strict';

// Register service worker and background sync
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/pwa/sw.js', { scope: '/pwa/' })
    .then(function(reg) {
      console.log('SW registered:', reg.scope);
      // Register background sync tag so sw.js sync handler fires on reconnect
      if ('sync' in reg) {
        reg.sync.register('smackerel-sync').catch(function() {
          // Background Sync API not supported — queue will rely on periodic flush
        });
      }
    })
    .catch(function(err) { console.error('SW registration failed:', err); });
}

// Handle install prompt
var deferredPrompt = null;
window.addEventListener('beforeinstallprompt', function(e) {
  e.preventDefault();
  deferredPrompt = e;
  var card = document.getElementById('install-card');
  if (card) { card.style.display = 'block'; }
});

var installBtn = document.getElementById('install-btn');
if (installBtn) {
  installBtn.addEventListener('click', function() {
    if (deferredPrompt) {
      deferredPrompt.prompt();
      deferredPrompt.userChoice.then(function() {
        deferredPrompt = null;
        var card = document.getElementById('install-card');
        if (card) { card.style.display = 'none'; }
      });
    }
  });
}

// ---------------------------------------------------------------------------
// PWA Settings retired (spec 100 SCOPE-03)
// ---------------------------------------------------------------------------
// The legacy "Server URL + Auth Token" settings surface was removed. The PWA is
// same-origin with the server and authenticates via the HttpOnly auth_token
// cookie set by /login; there is no token to paste and no localStorage
// credential state. Capture, share, and offline sync all rely on the cookie
// (same-origin fetches with credentials).
