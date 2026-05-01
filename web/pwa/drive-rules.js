// drive-rules.js — Spec 038 Scope 5 Screen 7
//
// Loads the Save Rules list, recent save requests, and audit feed for the
// drive write-back UI. All requests use the same Bearer token cookie/header
// pattern as the rest of the PWA.

(function () {
  'use strict';

  function authHeader() {
    var token = window.localStorage.getItem('smackerel-auth-token') || '';
    return token ? { Authorization: 'Bearer ' + token } : {};
  }

  function escapeHTML(s) {
    if (s == null) return '';
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function loadRules() {
    return fetch('/v1/drive/rules', { headers: authHeader() })
      .then(function (r) {
        if (!r.ok) throw new Error('rules HTTP ' + r.status);
        return r.json();
      })
      .then(function (body) {
        var rules = body.rules || [];
        var tbody = document.getElementById('rules-tbody');
        var table = document.getElementById('rules-table');
        var empty = document.getElementById('rules-empty');
        if (rules.length === 0) {
          empty.hidden = false;
          table.hidden = true;
          return rules;
        }
        empty.hidden = true;
        table.hidden = false;
        tbody.innerHTML = '';
        rules.forEach(function (rule) {
          var tr = document.createElement('tr');
          tr.innerHTML =
            '<td>' + escapeHTML(rule.name) + '</td>' +
            '<td>' + escapeHTML((rule.source_kinds || []).join(', ')) + '</td>' +
            '<td>' + escapeHTML(rule.classification) + '</td>' +
            '<td>' + escapeHTML(rule.provider_id) + '</td>' +
            '<td><code>' + escapeHTML(rule.target_folder_template) + '</code></td>' +
            '<td>' + (rule.enabled ? '<span class="badge badge-ok">enabled</span>' : '<span class="badge badge-warn">disabled</span>') + '</td>' +
            '<td><a href="/pwa/drive-rule-edit.html?id=' + encodeURIComponent(rule.id) + '">Edit</a></td>';
          tbody.appendChild(tr);
        });
        return rules;
      })
      .catch(function (err) {
        document.getElementById('rules-empty').textContent = 'Error loading rules: ' + err.message;
        document.getElementById('rules-empty').hidden = false;
      });
  }

  function loadAttempts() {
    return fetch('/v1/drive/save/requests?limit=50', { headers: authHeader() })
      .then(function (r) {
        if (!r.ok) throw new Error('save requests HTTP ' + r.status);
        return r.json();
      })
      .then(function (body) {
        var rows = body.requests || [];
        var tbody = document.getElementById('attempts-tbody');
        var table = document.getElementById('attempts-table');
        var empty = document.getElementById('attempts-empty');
        if (rows.length === 0) {
          empty.hidden = false;
          table.hidden = true;
          return;
        }
        empty.hidden = true;
        table.hidden = false;
        tbody.innerHTML = '';
        rows.forEach(function (row) {
          var tr = document.createElement('tr');
          var statusBadge = row.status === 'written'
            ? '<span class="badge badge-ok">written</span>'
            : (row.status === 'failed' ? '<span class="badge badge-err">failed</span>' : '<span class="badge">' + escapeHTML(row.status) + '</span>');
          var errCell = row.last_error
            ? '<td class="err-cell">' + escapeHTML(row.last_error) + '</td>'
            : '<td></td>';
          tr.innerHTML =
            '<td>' + escapeHTML(row.created_at) + '</td>' +
            '<td>' + escapeHTML(row.rule_id) + '</td>' +
            '<td>' + escapeHTML(row.source_artifact_id) + '</td>' +
            '<td>' + escapeHTML(row.target_path) + '</td>' +
            '<td>' + statusBadge + '</td>' +
            '<td>' + escapeHTML(row.attempts) + '</td>' +
            errCell;
          tbody.appendChild(tr);
        });
      })
      .catch(function (err) {
        document.getElementById('attempts-empty').textContent = 'Error loading attempts: ' + err.message;
        document.getElementById('attempts-empty').hidden = false;
      });
  }

  function loadAudit() {
    return fetch('/v1/drive/rules/audit?limit=100', { headers: authHeader() })
      .then(function (r) {
        if (!r.ok) throw new Error('audit HTTP ' + r.status);
        return r.json();
      })
      .then(function (body) {
        var rows = body.rows || [];
        var ul = document.getElementById('audit-list');
        var conflictsUl = document.getElementById('conflicts-list');
        var empty = document.getElementById('audit-empty');
        var conflictsEmpty = document.getElementById('conflicts-empty');
        ul.innerHTML = '';
        conflictsUl.innerHTML = '';
        var conflictCount = 0;
        if (rows.length === 0) {
          empty.hidden = false;
          conflictsEmpty.hidden = false;
          return;
        }
        empty.hidden = true;
        rows.forEach(function (row) {
          var li = document.createElement('li');
          li.className = 'audit-row audit-' + row.outcome;
          li.innerHTML =
            '<time>' + escapeHTML(row.created_at) + '</time> ' +
            '<span class="audit-outcome">' + escapeHTML(row.outcome) + '</span> ' +
            '<span class="audit-rule">' + escapeHTML(row.rule_id) + '</span> ' +
            '<span class="audit-artifact">' + escapeHTML(row.source_artifact_id) + '</span> ' +
            '<span class="audit-reason">' + escapeHTML(row.reason) + '</span>';
          ul.appendChild(li);
          if (row.outcome === 'conflict') {
            conflictCount++;
            var cli = document.createElement('li');
            cli.className = 'conflict-row';
            cli.textContent = row.created_at + ' — rule ' + row.rule_id + ' conflicted on artifact ' + row.source_artifact_id + ' (' + row.reason + ')';
            conflictsUl.appendChild(cli);
          }
        });
        conflictsEmpty.hidden = conflictCount > 0;
      })
      .catch(function (err) {
        document.getElementById('audit-empty').textContent = 'Error loading audit: ' + err.message;
        document.getElementById('audit-empty').hidden = false;
      });
  }

  document.addEventListener('DOMContentLoaded', function () {
    loadRules();
    loadAttempts();
    loadAudit();
  });
})();
