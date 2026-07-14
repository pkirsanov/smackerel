// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Spec 102 SCOPE-102-02 — alertmanager -> ntfy templating bridge test.
//
// SCN-102-C2-03: an Alertmanager webhook payload MUST be republished to ntfy as
// a TITLED, PRIORITY-TAGGED message (X-Title / X-Priority / X-Tags derived from
// the alert severity label + summary/description annotations) — NOT the raw
// Alertmanager JSON body.
//
// The adversarial edge is baked into the primary assertion: the test proves the
// ntfy body is the human-readable summary and NOT the raw webhook JSON (no
// `"version"` / `"alerts"` / `{` blob), so a regression that forwarded the raw
// payload straight through would fail here.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// capturedNtfy records what the stub ntfy endpoint received from the bridge.
type capturedNtfy struct {
	title    string
	priority string
	tags     string
	body     string
	count    int
}

func TestBridge_TitledPriorityNtfyRequest_Spec102(t *testing.T) {
	var cap capturedNtfy
	// Stub ntfy endpoint: captures the templated publish the bridge sends.
	ntfy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("stub ntfy: expected POST, got %s", r.Method)
		}
		cap.title = r.Header.Get("X-Title")
		cap.priority = r.Header.Get("X-Priority")
		cap.tags = r.Header.Get("X-Tags")
		bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		cap.body = string(bodyBytes)
		cap.count++
		w.WriteHeader(http.StatusOK)
	}))
	defer ntfy.Close()

	b := &bridge{ntfyURL: ntfy.URL, client: &http.Client{Timeout: 5 * time.Second}}

	// A realistic Alertmanager webhook v4 payload: one critical alert with a
	// summary annotation and a component label.
	payload := `{
	  "version": "4",
	  "status": "firing",
	  "receiver": "ntfy-self-hosted-alerts",
	  "alerts": [
	    {
	      "status": "firing",
	      "labels": {"alertname": "SmackerelBackupStale", "severity": "critical", "component": "backup"},
	      "annotations": {"summary": "No successful backup in over 26 hours", "description": "smackerel_backup_last_success_unixtime is stale"},
	      "startsAt": "2026-07-09T00:00:00Z"
	    }
	  ]
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	b.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("bridge handler returned HTTP %d (body=%q), want 200", rec.Code, rec.Body.String())
	}
	if cap.count != 1 {
		t.Fatalf("stub ntfy received %d publishes, want exactly 1", cap.count)
	}

	// X-Title: derived from status + alertname (NOT raw JSON).
	if !strings.Contains(cap.title, "SmackerelBackupStale") {
		t.Errorf("X-Title=%q does not contain the alertname 'SmackerelBackupStale'", cap.title)
	}
	if !strings.Contains(strings.ToUpper(cap.title), "FIRING") {
		t.Errorf("X-Title=%q does not carry the FIRING status word", cap.title)
	}

	// X-Priority: critical severity => ntfy max priority 5.
	if cap.priority != "5" {
		t.Errorf("X-Priority=%q, want \"5\" for severity=critical", cap.priority)
	}

	// X-Tags: severity + component derived.
	if !strings.Contains(cap.tags, "rotating_light") {
		t.Errorf("X-Tags=%q does not contain the critical-severity tag 'rotating_light'", cap.tags)
	}
	if !strings.Contains(cap.tags, "backup") {
		t.Errorf("X-Tags=%q does not contain the component tag 'backup'", cap.tags)
	}

	// Body: the human-readable summary, NOT the raw Alertmanager JSON.
	if !strings.Contains(cap.body, "No successful backup in over 26 hours") {
		t.Errorf("ntfy body=%q does not contain the alert summary", cap.body)
	}
	if strings.Contains(cap.body, "\"version\"") || strings.Contains(cap.body, "\"alerts\"") || strings.HasPrefix(strings.TrimSpace(cap.body), "{") {
		t.Errorf("adversarial: ntfy body=%q is the RAW Alertmanager JSON — the bridge MUST template, not forward the raw payload (SCN-102-C2-03)", cap.body)
	}
}

// TestBridge_SeverityPriorityMapping_Spec102 locks the severity->priority/tag
// mapping so a regression that flattens all alerts to one priority is caught.
func TestBridge_SeverityPriorityMapping_Spec102(t *testing.T) {
	cases := []struct {
		severity     string
		wantPriority string
		wantTag      string
	}{
		{"critical", "5", "rotating_light"},
		{"warning", "4", "warning"},
		{"info", "2", "information_source"},
		{"", "3", "bell"}, // unknown/absent severity => ntfy default priority
	}
	for _, tc := range cases {
		t.Run("severity="+tc.severity, func(t *testing.T) {
			msg := templateAlert(amAlert{
				Status:      "firing",
				Labels:      map[string]string{"alertname": "X", "severity": tc.severity},
				Annotations: map[string]string{"summary": "s"},
			})
			if msg.Priority != tc.wantPriority {
				t.Errorf("severity=%q -> priority=%q, want %q", tc.severity, msg.Priority, tc.wantPriority)
			}
			if !strings.Contains(msg.Tags, tc.wantTag) {
				t.Errorf("severity=%q -> tags=%q, want to contain %q", tc.severity, msg.Tags, tc.wantTag)
			}
		})
	}
}

// TestBridge_ResolvedIsLowPriority_Spec102 proves a resolved alert becomes a
// low-priority check-marked message regardless of its severity.
func TestBridge_ResolvedIsLowPriority_Spec102(t *testing.T) {
	msg := templateAlert(amAlert{
		Status:      "resolved",
		Labels:      map[string]string{"alertname": "SmackerelBackupStale", "severity": "critical", "component": "backup"},
		Annotations: map[string]string{"summary": "Backup recovered"},
	})
	if msg.Priority != "2" {
		t.Errorf("resolved critical alert priority=%q, want \"2\" (good news is low priority)", msg.Priority)
	}
	if !strings.Contains(msg.Tags, "white_check_mark") {
		t.Errorf("resolved alert tags=%q, want to contain 'white_check_mark'", msg.Tags)
	}
	if !strings.Contains(strings.ToUpper(msg.Title), "RESOLVED") {
		t.Errorf("resolved alert title=%q, want to carry RESOLVED", msg.Title)
	}
}

// TestBridge_NewNtfyRequestHeaders_Spec102 asserts the low-level request builder
// sets exactly the ntfy publish headers.
func TestBridge_NewNtfyRequestHeaders_Spec102(t *testing.T) {
	m := ntfyMessage{Title: "T", Priority: "4", Tags: "warning,backup", Body: "hello"}
	req, err := newNtfyRequest("http://ntfy.example/topic", m)
	if err != nil {
		t.Fatalf("newNtfyRequest: %v", err)
	}
	if got := req.Header.Get("X-Title"); got != "T" {
		t.Errorf("X-Title=%q, want T", got)
	}
	if got := req.Header.Get("X-Priority"); got != "4" {
		t.Errorf("X-Priority=%q, want 4", got)
	}
	if got := req.Header.Get("X-Tags"); got != "warning,backup" {
		t.Errorf("X-Tags=%q, want warning,backup", got)
	}
	body, _ := io.ReadAll(req.Body)
	if string(body) != "hello" {
		t.Errorf("body=%q, want hello", string(body))
	}
	// Ensure the body is NOT JSON (defensive parity with the templating rule).
	var js map[string]any
	if json.Unmarshal(bytes.TrimSpace(body), &js) == nil {
		t.Errorf("ntfy body parsed as JSON object %v — templated body must be plain text", js)
	}
}
