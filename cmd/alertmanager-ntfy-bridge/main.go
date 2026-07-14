// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Command alertmanager-ntfy-bridge is the spec 102 SCOPE-102-02 templating
// bridge that sits between the in-stack Alertmanager and the operator's ntfy
// topic.
//
// # WHY IT EXISTS
//
// Alertmanager's webhook receiver can only POST its raw JSON body to a URL; it
// cannot set ntfy's per-message X-Title / X-Priority / X-Tags headers. Without
// this bridge, a fired alert would arrive at ntfy as an unreadable JSON blob.
// The bridge receives the Alertmanager webhook payload and re-publishes each
// alert to ntfy as a TITLED, PRIORITY-TAGGED message derived from the alert's
// severity label and summary/description annotations (SCN-102-C2-03).
//
// # WHY IT RIDES THE CORE IMAGE
//
// It is built into the ALREADY-pinned smackerel-core image (the Dockerfile
// builds ./cmd/alertmanager-ntfy-bridge alongside ./cmd/core), so there is NO
// new external image to pin/sign (design OQ-102-1 default). The deploy compose
// runs it as a monitoring-profiled service with the entrypoint overridden to
// this binary.
//
// SECRET / ENV-SPECIFIC BOUNDARY
//
// The OPERATOR-PRIVATE ntfy endpoint is read fail-loud from ALERTMANAGER_NTFY_URL
// (adapter-injected at apply — No env-specific content in the product repo). The
// in-network listen address is ALERTMANAGER_BRIDGE_LISTEN_ADDR. Both are REQUIRED
// with NO default (smackerel-no-defaults / Gate G028): an empty value aborts at
// boot, it is never silently defaulted.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// maxWebhookBytes caps the Alertmanager webhook body the bridge will read, so a
// malformed or hostile POST cannot exhaust memory. Alertmanager payloads are a
// few KB even for large groups; 1 MiB is generous.
const maxWebhookBytes = 1 << 20

// amWebhook is the subset of the Alertmanager webhook (schema version 4) the
// bridge reads. Unlisted fields are ignored.
type amWebhook struct {
	Version  string    `json:"version"`
	Status   string    `json:"status"`
	Receiver string    `json:"receiver"`
	Alerts   []amAlert `json:"alerts"`
}

// amAlert is one alert inside the webhook's alerts[] array.
type amAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	EndsAt      string            `json:"endsAt"`
}

// ntfyMessage is the templated ntfy publish derived from ONE alert. It is a
// pure value so templateAlert can be unit-tested without any HTTP.
type ntfyMessage struct {
	Title    string
	Priority string
	Tags     string
	Body     string
}

// firstNonEmpty returns the first trimmed non-empty string in vs, or "".
func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

// priorityAndTagForSeverity maps an Alertmanager `severity` label to an ntfy
// priority (1=min .. 5=max) and a representative ntfy tag (emoji shortcode).
// An unknown/empty severity falls back to the ntfy default priority (3) — this
// is NOT a smackerel-no-defaults violation: it is a display-only mapping of an
// UPSTREAM-owned free-form label, not a substitute for a missing REQUIRED
// runtime config value.
func priorityAndTagForSeverity(severity string) (priority, tag string) {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return "5", "rotating_light"
	case "warning":
		return "4", "warning"
	case "info", "information", "informational":
		return "2", "information_source"
	default:
		return "3", "bell"
	}
}

// sanitizeTag reduces an arbitrary label value to an ntfy-tag-safe token
// (ntfy tags are comma-separated; commas and whitespace are collapsed).
func sanitizeTag(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", " ")
	return strings.Join(strings.Fields(s), "_")
}

// templateAlert turns ONE Alertmanager alert into a titled/priority-tagged ntfy
// message. This is the TEMPLATING the task requires: ntfy renders a readable
// notification, never the raw Alertmanager JSON body.
func templateAlert(a amAlert) ntfyMessage {
	alertname := firstNonEmpty(a.Labels["alertname"], "alert")
	component := strings.TrimSpace(a.Labels["component"])
	summary := firstNonEmpty(a.Annotations["summary"], a.Annotations["description"], alertname)

	statusWord := "FIRING"
	resolved := strings.EqualFold(strings.TrimSpace(a.Status), "resolved")
	if resolved {
		statusWord = "RESOLVED"
	}

	title := statusWord + " · " + alertname
	if component != "" {
		title += " (" + component + ")"
	}

	priority, sevTag := priorityAndTagForSeverity(a.Labels["severity"])
	tags := sevTag
	if resolved {
		// A resolved alert is good news regardless of severity: drop it to a
		// low priority and mark it with a check.
		priority = "2"
		tags = "white_check_mark"
	}
	if component != "" {
		tags += "," + sanitizeTag(component)
	}

	return ntfyMessage{
		Title:    title,
		Priority: priority,
		Tags:     tags,
		Body:     summary,
	}
}

// newNtfyRequest builds the ntfy publish HTTP request for a templated message.
func newNtfyRequest(ntfyURL string, m ntfyMessage) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, ntfyURL, strings.NewReader(m.Body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Title", m.Title)
	req.Header.Set("X-Priority", m.Priority)
	req.Header.Set("X-Tags", m.Tags)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	return req, nil
}

// bridge holds the runtime dependencies (the ntfy endpoint + an HTTP client).
type bridge struct {
	ntfyURL string
	client  *http.Client
}

// handleWebhook is the Alertmanager webhook receiver. It fans each alert in the
// payload out to ntfy as a templated message.
func (b *bridge) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBytes))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	var hook amWebhook
	if err := json.Unmarshal(raw, &hook); err != nil {
		http.Error(w, "invalid alertmanager webhook JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(hook.Alerts) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var firstErr error
	delivered := 0
	for _, a := range hook.Alerts {
		msg := templateAlert(a)
		req, err := newNtfyRequest(b.ntfyURL, msg)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		resp, err := b.client.Do(req)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// Drain + close so the connection can be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 {
			if firstErr == nil {
				firstErr = fmt.Errorf("ntfy returned HTTP %d", resp.StatusCode)
			}
			continue
		}
		delivered++
	}

	if firstErr != nil {
		http.Error(w, fmt.Sprintf("delivered %d/%d alerts; first error: %v", delivered, len(hook.Alerts), firstErr), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "delivered %d alert(s) to ntfy (templated)\n", delivered)
}

func main() {
	log.SetFlags(0)

	ntfyURL := strings.TrimSpace(os.Getenv("ALERTMANAGER_NTFY_URL"))
	if ntfyURL == "" {
		log.Fatal("alertmanager-ntfy-bridge: ALERTMANAGER_NTFY_URL is required (operator-private ntfy endpoint; no default) — fail-loud per smackerel-no-defaults / Gate G028")
	}
	listenAddr := strings.TrimSpace(os.Getenv("ALERTMANAGER_BRIDGE_LISTEN_ADDR"))
	if listenAddr == "" {
		log.Fatal("alertmanager-ntfy-bridge: ALERTMANAGER_BRIDGE_LISTEN_ADDR is required (no default) — fail-loud per smackerel-no-defaults / Gate G028")
	}

	b := &bridge{
		ntfyURL: ntfyURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("/", b.handleWebhook)

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("alertmanager-ntfy-bridge: listening on %s, forwarding templated alerts to ntfy", listenAddr)
	log.Fatal(srv.ListenAndServe())
}
