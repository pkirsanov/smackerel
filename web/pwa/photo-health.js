// Spec 040 Scope 5 — Photo Health dashboard.
//
// Renders LIVE numbers from `/v1/photos/health`. Capability limit
// banners read the canonical `limitation_code` returned by the
// capability taxonomy registry. The static `<li data-limitation-code>`
// anchors in `photo-health.html` are the canary the integration test
// `TestPhotosCapabilityTaxonomyCanary_GoRegistryMatchesPWALimitationCodes`
// inspects to prove the Go registry stays in sync with the PWA.

(function loadPhotoHealth() {
  const summarySection = document.getElementById('photo-health-summary');
  const status = document.getElementById('photo-health-status');
  const errorBox = document.getElementById('photo-health-error');
  const counts = document.getElementById('photo-health-counts');
  const lifecycleEl = document.getElementById('photo-health-lifecycle');
  const duplicatesTotalEl = document.getElementById('photo-health-duplicates-total');
  const removalPendingEl = document.getElementById('photo-health-removal-pending');
  const capabilityStatus = document.getElementById('photo-health-capability-status');
  const capabilityList = document.getElementById('photo-health-capability-list');
  const skipsStatus = document.getElementById('photo-health-skips-status');
  const skipsList = document.getElementById('photo-health-skips-list');

  fetch('/v1/photos/health', { credentials: 'same-origin' })
    .then(function handle(response) {
      if (!response.ok) {
        throw new Error('photo health endpoint returned HTTP ' + response.status);
      }
      return response.json();
    })
    .then(function render(body) {
      // Summary block.
      const lifecycle = body.lifecycle || {};
      const lifecycleStates = lifecycle.states || lifecycle.lifecycle_states || lifecycle;
      lifecycleEl.textContent = formatStates(lifecycleStates);
      const duplicates = body.duplicates || {};
      duplicatesTotalEl.textContent = String(duplicates.total || 0);
      removalPendingEl.textContent = String(body.removal_pending || 0);
      counts.hidden = false;
      status.textContent = 'Loaded live photo health.';
      summarySection.setAttribute('aria-busy', 'false');

      // Capability limits block.
      const limits = body.capability_limits || [];
      capabilityList.innerHTML = '';
      if (limits.length === 0) {
        capabilityStatus.textContent = 'No capability limits registered.';
      } else {
        limits.forEach(function appendLimit(limit) {
          const item = document.createElement('li');
          item.setAttribute('data-limitation-code', limit.limitation_code);
          item.setAttribute('data-capability', limit.capability);
          item.setAttribute('data-status', limit.status);
          const title = document.createElement('strong');
          title.textContent = limit.banner_title || limit.capability;
          item.appendChild(title);
          item.appendChild(document.createTextNode(' — ' + (limit.banner_body || '')));
          capabilityList.appendChild(item);
        });
        capabilityStatus.textContent = 'Loaded ' + limits.length + ' capability limit entries.';
        capabilityList.hidden = false;
      }

      // Skip ledger block.
      const skips = body.skips || [];
      skipsList.innerHTML = '';
      if (skips.length === 0) {
        skipsStatus.textContent = 'No provider skips reported.';
      } else {
        skips.forEach(function appendSkip(skip) {
          const item = document.createElement('li');
          item.setAttribute('data-skip-reason', skip.reason);
          item.setAttribute('data-provider', skip.provider);
          item.textContent = skip.provider + ': ' + skip.reason + ' (' + skip.count + ')';
          skipsList.appendChild(item);
        });
        skipsStatus.textContent = 'Loaded ' + skips.length + ' skip entries.';
        skipsList.hidden = false;
      }
    })
    .catch(function handleError(err) {
      status.textContent = '';
      errorBox.textContent = 'Failed to load photo health: ' + err.message;
      errorBox.hidden = false;
      summarySection.setAttribute('aria-busy', 'false');
    });

  function formatStates(states) {
    if (!states || typeof states !== 'object') {
      return '0';
    }
    const parts = [];
    Object.keys(states).forEach(function appendState(key) {
      const value = states[key];
      if (typeof value === 'number') {
        parts.push(key + ': ' + value);
      }
    });
    return parts.length === 0 ? '0' : parts.join(', ');
  }
})();
