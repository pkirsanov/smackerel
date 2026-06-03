// Package api — spec 027 scope 9 PLAN-9-04 source_channel header.
//
// The browser/PWA UI (spec 073) declares the source channel on every
// annotation write via the X-Smackerel-Source request header. The
// allowlist is closed-set per the NO-DEFAULTS SST policy: missing or
// unknown values produce a fail-loud 400 (no fallback, no default).
//
// Telegram and extension handlers do NOT use this header — they call
// the store with a hardcoded channel through their adapter path. The
// header contract applies only to handlers mounted under the
// annotation router.
package api

import (
	"net/http"

	"github.com/smackerel/smackerel/internal/annotation"
)

// AnnotationSourceHeader is the request-header name carrying the
// channel declaration. Must stay in lockstep with
// config/smackerel.yaml → annotations.source_header_name.
const AnnotationSourceHeader = "X-Smackerel-Source"

// allowedAnnotationSources is the closed-set allowlist. Must stay in
// lockstep with config/smackerel.yaml → annotations.source_allowlist
// (spec 027 scope 9 PLAN-9-04).
var allowedAnnotationSources = []annotation.SourceChannel{
	annotation.ChannelWeb,
	annotation.ChannelExtension,
	annotation.ChannelTelegram,
	annotation.ChannelAPI,
}

// resolveAnnotationSource returns the validated channel or writes a
// 400 to w and returns ("", false) when the header is missing or
// carries an unknown value.
func resolveAnnotationSource(w http.ResponseWriter, r *http.Request) (annotation.SourceChannel, bool) {
	raw := r.Header.Get(AnnotationSourceHeader)
	if raw == "" {
		http.Error(w, `{"error":"X-Smackerel-Source header required"}`, http.StatusBadRequest)
		return "", false
	}
	candidate := annotation.SourceChannel(raw)
	for _, allowed := range allowedAnnotationSources {
		if candidate == allowed {
			return candidate, true
		}
	}
	http.Error(w, `{"error":"unknown source_channel"}`, http.StatusBadRequest)
	return "", false
}
