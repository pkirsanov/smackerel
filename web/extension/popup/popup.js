// Smackerel Extension Popup — Setup, Capture & Validation
'use strict';

// ---------------------------------------------------------------------------
// DOM References
// ---------------------------------------------------------------------------

var setupScreen = document.getElementById('setup-screen');
var mainScreen = document.getElementById('main-screen');
var serverUrlInput = document.getElementById('server-url');
var authTokenInput = document.getElementById('auth-token');
var httpWarning = document.getElementById('http-warning');
var testBtn = document.getElementById('test-btn');
var testResult = document.getElementById('test-result');
var saveBtn = document.getElementById('save-btn');
var connectionStatus = document.getElementById('connection-status');
var pageTitle = document.getElementById('page-title');
var pageUrl = document.getElementById('page-url');
var captureBtn = document.getElementById('capture-btn');
var captureResult = document.getElementById('capture-result');
var queueStatus = document.getElementById('queue-status');
var queueCount = document.getElementById('queue-count');
var syncBtn = document.getElementById('sync-btn');
var settingsBtn = document.getElementById('settings-btn');

// ---------------------------------------------------------------------------
// Init: check if already configured
// ---------------------------------------------------------------------------

chrome.storage.local.get(['serverUrl', 'authToken'], function(data) {
  if (data.serverUrl && data.authToken) {
    showMainScreen(data.serverUrl, data.authToken);
  } else {
    setupScreen.classList.remove('hidden');
    mainScreen.classList.add('hidden');
  }
});

// ---------------------------------------------------------------------------
// Setup Screen — Server URL HTTP Warning (Scope 7)
// ---------------------------------------------------------------------------

serverUrlInput.addEventListener('input', function() {
  var url = serverUrlInput.value.trim();
  if (url && url.startsWith('http://')) {
    httpWarning.classList.remove('hidden');
  } else {
    httpWarning.classList.add('hidden');
  }
});

// ---------------------------------------------------------------------------
// Test Connection (Scope 7)
// ---------------------------------------------------------------------------

testBtn.addEventListener('click', function() {
  var serverUrl = serverUrlInput.value.trim().replace(/\/+$/, '');
  var authToken = authTokenInput.value.trim();

  if (!serverUrl) {
    showTestResult('error', '❌ Server URL is required');
    return;
  }
  if (!authToken) {
    showTestResult('error', '❌ Auth token is required');
    return;
  }

  testBtn.disabled = true;
  testBtn.textContent = 'Testing...';
  testResult.classList.add('hidden');

  fetch(serverUrl + '/api/health', {
    method: 'GET',
    headers: { 'Authorization': 'Bearer ' + authToken }
  })
  .then(function(resp) {
    if (resp.ok) {
      showTestResult('success', '✅ Connected');
      saveBtn.classList.remove('hidden');
    } else if (resp.status === 401) {
      showTestResult('error', '❌ Authentication failed — check your token');
      saveBtn.classList.add('hidden');
    } else {
      showTestResult('error', '❌ Server returned ' + resp.status);
      saveBtn.classList.add('hidden');
    }
  })
  .catch(function(err) {
    showTestResult('error', '❌ Connection failed — check server URL');
    saveBtn.classList.add('hidden');
  })
  .finally(function() {
    testBtn.disabled = false;
    testBtn.textContent = 'Test Connection';
  });
});

function showTestResult(type, message) {
  testResult.className = 'status-box ' + type;
  testResult.textContent = message;
  testResult.classList.remove('hidden');
}

// ---------------------------------------------------------------------------
// Save Settings (Scope 7 — only after successful validation)
// ---------------------------------------------------------------------------

saveBtn.addEventListener('click', function() {
  var serverUrl = serverUrlInput.value.trim().replace(/\/+$/, '');
  var authToken = authTokenInput.value.trim();

  chrome.storage.local.set({
    serverUrl: serverUrl,
    authToken: authToken
  }, function() {
    showMainScreen(serverUrl, authToken);
  });
});

// ---------------------------------------------------------------------------
// Main Screen
// ---------------------------------------------------------------------------

function showMainScreen(serverUrl, authToken) {
  setupScreen.classList.add('hidden');
  mainScreen.classList.remove('hidden');

  // Show current tab info (Scope 3)
  chrome.tabs.query({ active: true, currentWindow: true }, function(tabs) {
    if (tabs[0]) {
      pageTitle.textContent = tabs[0].title || 'Untitled';
      pageUrl.textContent = tabs[0].url || '';
    }
  });

  // Check connection status (Scope 7)
  connectionStatus.textContent = 'Checking...';
  connectionStatus.className = 'connection-status';

  fetch(serverUrl.replace(/\/+$/, '') + '/api/health', {
    method: 'GET',
    headers: { 'Authorization': 'Bearer ' + authToken }
  })
  .then(function(resp) {
    if (resp.ok) {
      connectionStatus.textContent = '● Connected';
      connectionStatus.className = 'connection-status connected';
    } else {
      connectionStatus.textContent = '● Disconnected';
      connectionStatus.className = 'connection-status disconnected';
    }
  })
  .catch(function() {
    connectionStatus.textContent = '● Offline';
    connectionStatus.className = 'connection-status disconnected';
  });

  // Check queue count (Scope 5)
  chrome.runtime.sendMessage({ action: 'getQueueCount' }, function(resp) {
    if (resp && resp.count > 0) {
      queueCount.textContent = resp.count;
      queueStatus.classList.remove('hidden');
    } else {
      queueStatus.classList.add('hidden');
    }
  });
}

// ---------------------------------------------------------------------------
// Capture Button (Scope 3)
// ---------------------------------------------------------------------------

captureBtn.addEventListener('click', function() {
  captureBtn.disabled = true;
  captureBtn.textContent = 'Saving...';
  captureResult.classList.add('hidden');

  chrome.tabs.query({ active: true, currentWindow: true }, function(tabs) {
    var tab = tabs[0];
    if (!tab) {
      showCaptureResult('error', '❌ No active tab');
      captureBtn.disabled = false;
      captureBtn.textContent = 'Save to Smackerel';
      return;
    }

    chrome.runtime.sendMessage({
      action: 'capture',
      data: {
        url: tab.url,
        title: tab.title,
        text: '' // no selection from toolbar click
      }
    }, function(result) {
      if (result && result.success) {
        if (result.queued) {
          showCaptureResult('info', '📱 Saved offline — will sync when connected');
        } else {
          showCaptureResult('success', '✅ Saved!');
        }
      } else if (result && result.error === 'not_configured') {
        showCaptureResult('error', '❌ Please configure Smackerel first');
        // Switch to setup screen
        setupScreen.classList.remove('hidden');
        mainScreen.classList.add('hidden');
      } else {
        showCaptureResult('error', '❌ Save failed');
      }
      captureBtn.disabled = false;
      captureBtn.textContent = 'Save to Smackerel';
    });
  });
});

function showCaptureResult(type, message) {
  captureResult.className = 'status-box ' + type;
  captureResult.textContent = message;
  captureResult.classList.remove('hidden');
}

// ---------------------------------------------------------------------------
// Sync Button (Scope 5)
// ---------------------------------------------------------------------------

syncBtn.addEventListener('click', function() {
  syncBtn.textContent = 'Syncing...';
  chrome.runtime.sendMessage({ action: 'flushQueue' }, function(result) {
    syncBtn.textContent = 'Sync now';
    if (result) {
      if (result.authFailed) {
        showCaptureResult('error', '❌ Auth token expired — update in settings');
      } else if (result.flushed > 0) {
        showCaptureResult('success', '✅ Synced ' + result.flushed + ' item(s)');
      }
      // Refresh queue count
      chrome.runtime.sendMessage({ action: 'getQueueCount' }, function(resp) {
        if (resp && resp.count > 0) {
          queueCount.textContent = resp.count;
          queueStatus.classList.remove('hidden');
        } else {
          queueStatus.classList.add('hidden');
        }
      });
    }
  });
});

// ---------------------------------------------------------------------------
// Settings Button — switch back to setup screen
// ---------------------------------------------------------------------------

settingsBtn.addEventListener('click', function() {
  chrome.storage.local.get(['serverUrl', 'authToken'], function(data) {
    serverUrlInput.value = data.serverUrl || '';
    authTokenInput.value = data.authToken || '';
    setupScreen.classList.remove('hidden');
    mainScreen.classList.add('hidden');
    saveBtn.classList.add('hidden');
    testResult.classList.add('hidden');
  });
});
