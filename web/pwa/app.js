// Smackerel PWA — App Shell Script
// Handles service worker registration, install prompt, and settings.
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

  // Respond to service worker config requests for offline queue flush
  navigator.serviceWorker.addEventListener('message', function(event) {
    if (event.data && event.data.type === 'getConfig') {
      var serverUrl = localStorage.getItem('smackerel_server_url') || '';
      var authToken = localStorage.getItem('smackerel_auth_token') || '';
      event.ports[0].postMessage({ serverUrl: serverUrl, authToken: authToken });
    }
  });
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
// PWA Settings — Server URL and Auth Token (GAP-1 fix)
// ---------------------------------------------------------------------------

(function() {
  var serverUrlInput = document.getElementById('pwa-server-url');
  var authTokenInput = document.getElementById('pwa-auth-token');
  var testBtn = document.getElementById('pwa-test-btn');
  var saveBtn = document.getElementById('pwa-save-btn');
  var statusEl = document.getElementById('pwa-settings-status');

  if (!serverUrlInput || !testBtn) return; // not on the settings page

  // Load saved values
  var savedUrl = localStorage.getItem('smackerel_server_url') || '';
  var savedToken = localStorage.getItem('smackerel_auth_token') || '';
  if (savedUrl) serverUrlInput.value = savedUrl;
  if (savedToken) authTokenInput.value = savedToken;

  function showStatus(type, msg) {
    statusEl.className = 'status ' + type;
    statusEl.textContent = msg;
    statusEl.classList.remove('hidden');
  }

  testBtn.addEventListener('click', function() {
    var url = serverUrlInput.value.trim().replace(/\/+$/, '');
    var token = authTokenInput.value.trim();
    if (!url) { showStatus('error', '❌ Server URL is required'); return; }
    if (!token) { showStatus('error', '❌ Auth token is required'); return; }

    testBtn.disabled = true;
    testBtn.textContent = 'Testing...';
    statusEl.classList.add('hidden');

    fetch(url + '/api/health', {
      method: 'GET',
      headers: { 'Authorization': 'Bearer ' + token }
    })
    .then(function(resp) {
      if (resp.ok) {
        showStatus('success', '✅ Connected');
        saveBtn.classList.remove('hidden');
      } else if (resp.status === 401) {
        showStatus('error', '❌ Authentication failed — check your token');
        saveBtn.classList.add('hidden');
      } else {
        showStatus('error', '❌ Server returned ' + resp.status);
        saveBtn.classList.add('hidden');
      }
    })
    .catch(function() {
      showStatus('error', '❌ Connection failed — check server URL');
      saveBtn.classList.add('hidden');
    })
    .finally(function() {
      testBtn.disabled = false;
      testBtn.textContent = 'Test Connection';
    });
  });

  saveBtn.addEventListener('click', function() {
    var url = serverUrlInput.value.trim().replace(/\/+$/, '');
    var token = authTokenInput.value.trim();
    localStorage.setItem('smackerel_server_url', url);
    localStorage.setItem('smackerel_auth_token', token);
    showStatus('success', '✅ Settings saved');
    saveBtn.classList.add('hidden');
  });
})();
