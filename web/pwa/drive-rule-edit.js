// drive-rule-edit.js — Spec 038 Scope 5 Screen 8

(function () {
  'use strict';

  function authHeader() {
    // Spec 100 SCOPE-03 — auth is the same-origin HttpOnly auth_token cookie
    // (sent automatically by the same-origin fetch); no bearer token in storage.
    return { 'Content-Type': 'application/json' };
  }

  function getQueryParam(name) {
    var params = new URLSearchParams(window.location.search);
    return params.get(name) || '';
  }

  function setStatus(msg, isError) {
    var el = document.getElementById('rule-status');
    el.textContent = msg;
    el.className = isError ? 'status status-error' : 'status status-info';
  }

  function getCheckedValues(name) {
    var checked = document.querySelectorAll('input[name="' + name + '"]:checked');
    var out = [];
    for (var i = 0; i < checked.length; i++) out.push(checked[i].value);
    return out;
  }

  function readForm() {
    return {
      name: document.getElementById('rule-name').value,
      enabled: document.getElementById('rule-enabled').checked,
      source_kinds: getCheckedValues('source_kinds'),
      classification: document.getElementById('rule-classification').value,
      sensitivity_in: getCheckedValues('sensitivity_in'),
      confidence_min: parseFloat(document.getElementById('rule-confidence-min').value || '0'),
      provider_id: document.getElementById('rule-provider-id').value,
      target_folder_template: document.getElementById('rule-target-template').value,
      on_missing_folder: document.getElementById('rule-on-missing-folder').value,
      on_existing_file: document.getElementById('rule-on-existing-file').value,
      guardrails: {
        never_link_share: document.getElementById('rule-guardrails-never-link-share').checked,
        require_confirm_below: parseFloat(document.getElementById('rule-guardrails-require-confirm-below').value || '0')
      }
    };
  }

  function fillForm(rule) {
    document.getElementById('rule-name').value = rule.name || '';
    document.getElementById('rule-enabled').checked = !!rule.enabled;
    var srcInputs = document.querySelectorAll('input[name="source_kinds"]');
    for (var i = 0; i < srcInputs.length; i++) {
      srcInputs[i].checked = (rule.source_kinds || []).indexOf(srcInputs[i].value) !== -1;
    }
    document.getElementById('rule-classification').value = rule.classification || '';
    var sensInputs = document.querySelectorAll('input[name="sensitivity_in"]');
    for (var j = 0; j < sensInputs.length; j++) {
      sensInputs[j].checked = (rule.sensitivity_in || []).indexOf(sensInputs[j].value) !== -1;
    }
    document.getElementById('rule-confidence-min').value = rule.confidence_min || 0;
    document.getElementById('rule-provider-id').value = rule.provider_id || '';
    document.getElementById('rule-target-template').value = rule.target_folder_template || '';
    document.getElementById('rule-on-missing-folder').value = rule.on_missing_folder || 'create';
    document.getElementById('rule-on-existing-file').value = rule.on_existing_file || 'version';
    document.getElementById('rule-guardrails-never-link-share').checked = !!(rule.guardrails && rule.guardrails.never_link_share);
    document.getElementById('rule-guardrails-require-confirm-below').value = (rule.guardrails && rule.guardrails.require_confirm_below) || 0;
  }

  function loadRule(id) {
    return fetch('/v1/drive/rules/' + encodeURIComponent(id), { headers: authHeader() })
      .then(function (r) {
        if (!r.ok) throw new Error('rule HTTP ' + r.status);
        return r.json();
      })
      .then(function (rule) {
        fillForm(rule);
        document.getElementById('page-heading').textContent = 'Edit rule — ' + rule.name;
        document.getElementById('rule-delete').hidden = false;
      })
      .catch(function (err) { setStatus('Error loading rule: ' + err.message, true); });
  }

  function saveRule(id) {
    var body = readForm();
    if (id) body.id = id;
    var url = id ? '/v1/drive/rules/' + encodeURIComponent(id) : '/v1/drive/rules';
    var method = id ? 'PUT' : 'POST';
    return fetch(url, { method: method, headers: authHeader(), body: JSON.stringify(body) })
      .then(function (r) {
        if (!r.ok) {
          return r.text().then(function (text) { throw new Error(r.status + ': ' + text); });
        }
        return r.json();
      })
      .then(function (saved) {
        setStatus('Saved.', false);
        if (!id && saved && saved.id) {
          window.location.replace('/pwa/drive-rule-edit.html?id=' + encodeURIComponent(saved.id));
        }
      })
      .catch(function (err) { setStatus('Save failed: ' + err.message, true); });
  }

  function deleteRule(id) {
    if (!id) return;
    if (!window.confirm('Delete this save rule? Existing audit rows are kept.')) return;
    fetch('/v1/drive/rules/' + encodeURIComponent(id), { method: 'DELETE', headers: authHeader() })
      .then(function (r) {
        if (!r.ok && r.status !== 204) throw new Error('delete HTTP ' + r.status);
        window.location.assign('/pwa/drive-rules.html');
      })
      .catch(function (err) { setStatus('Delete failed: ' + err.message, true); });
  }

  function runDryRun(id) {
    if (!id) {
      setStatus('Save the rule first before dry-running it.', true);
      return;
    }
    var tokensRaw = document.getElementById('dr-tokens').value || '{}';
    var tokens;
    try {
      tokens = JSON.parse(tokensRaw);
    } catch (e) {
      setStatus('Invalid tokens JSON: ' + e.message, true);
      return;
    }
    var body = {
      source_artifact_id: document.getElementById('dr-artifact-id').value,
      source_kind: document.getElementById('dr-source-kind').value,
      classification: document.getElementById('dr-classification').value,
      sensitivity: document.getElementById('dr-sensitivity').value,
      confidence: parseFloat(document.getElementById('dr-confidence').value || '0'),
      tokens: tokens,
      captured_at: document.getElementById('dr-captured-at').value
    };
    fetch('/v1/drive/rules/' + encodeURIComponent(id) + '/test', {
      method: 'POST', headers: authHeader(), body: JSON.stringify(body)
    })
      .then(function (r) { return r.json().then(function (j) { return { ok: r.ok, status: r.status, body: j }; }); })
      .then(function (result) {
        var pre = document.getElementById('dry-run-result');
        pre.hidden = false;
        pre.textContent = JSON.stringify(result.body, null, 2);
        if (!result.ok) setStatus('Dry-run HTTP ' + result.status, true);
      })
      .catch(function (err) { setStatus('Dry-run failed: ' + err.message, true); });
  }

  document.addEventListener('DOMContentLoaded', function () {
    var id = getQueryParam('id');
    if (id) loadRule(id);

    document.getElementById('rule-form').addEventListener('submit', function (ev) {
      ev.preventDefault();
      saveRule(id);
    });
    document.getElementById('rule-delete').addEventListener('click', function () { deleteRule(id); });
    document.getElementById('dry-run-form').addEventListener('submit', function (ev) {
      ev.preventDefault();
      runDryRun(id);
    });
  });
})();
