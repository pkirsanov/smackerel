// Smackerel PWA — App Shell Script
// Handles service worker registration and install prompt.
'use strict';

// Register service worker
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/pwa/sw.js', { scope: '/pwa/' })
    .then(function(reg) { console.log('SW registered:', reg.scope); })
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
