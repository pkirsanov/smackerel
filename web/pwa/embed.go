// Package pwa embeds the Progressive Web App static assets for serving
// by the Go HTTP server. See specs/033-mobile-capture for the feature spec.
package pwa

import "embed"

//go:embed *.html *.css *.js *.json *.svg lib
var StaticFiles embed.FS
