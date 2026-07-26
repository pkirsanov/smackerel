package experience

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// consumer_inventory.go inventories the REAL cross-surface navigation, manifest,
// service-worker, and catalog destinations and runs a blocking stale-reference
// scan. "Stale" means a consumer points at a destination that is not a real
// registered browser route and not a real PWA static asset — i.e. an invented
// or dead endpoint. The scan reads source directly (no live stack), so it runs
// under `./smackerel.sh check` / a focused `go test`.
//
// This slice ADDS the generated catalog alongside the still-active handwritten
// navigation authorities; it does not change them. The scan therefore validates
// BOTH the existing consumers (server nav, PWA nav, manifest, service worker)
// AND the newly-generated catalog's bound routes against the actual router and
// PWA file tree, proving no invented endpoints entered anywhere.

// ConsumerRef is one destination reference found in a real consumer surface.
type ConsumerRef struct {
	Consumer string // e.g. "server-nav", "pwa-nav", "manifest-shortcut", "service-worker", "catalog-href"
	Path     string
}

// repoRoot walks up from the current working directory to the directory that
// contains go.mod (the repository root).
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("experience: go.mod not found walking up from cwd")
		}
		dir = parent
	}
}

var (
	reServerNavHref = regexp.MustCompile(`href="(/[^"]*)"`)
	rePWANavHref    = regexp.MustCompile(`href:\s*"(/[^"]*)"`)
	reSWAsset       = regexp.MustCompile(`'(/[^']*)'`)
	reRouteLiteral  = regexp.MustCompile(`r\.(?:Get|Post|Put|Patch|Delete|Handle|Route)\(\s*"(/[^"]*)"`)
	reRouteMethod   = regexp.MustCompile(`r\.Method\([^,]+,\s*"(/[^"]*)"`)
)

func readFile(root string, rel ...string) (string, error) {
	p := filepath.Join(append([]string{root}, rel...)...)
	b, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("experience: read %s: %w", filepath.Join(rel...), err)
	}
	return string(b), nil
}

// inventoryServerNav parses the still-active server app-shell nav hrefs.
func inventoryServerNav(root string) ([]ConsumerRef, error) {
	src, err := readFile(root, "internal", "web", "appshell.go")
	if err != nil {
		return nil, err
	}
	var refs []ConsumerRef
	for _, m := range reServerNavHref.FindAllStringSubmatch(src, -1) {
		refs = append(refs, ConsumerRef{Consumer: "server-nav", Path: m[1]})
	}
	return refs, nil
}

// inventoryPWANav parses the still-active PWA app-shell nav hrefs.
func inventoryPWANav(root string) ([]ConsumerRef, error) {
	src, err := readFile(root, "web", "pwa", "lib", "appnav.js")
	if err != nil {
		return nil, err
	}
	var refs []ConsumerRef
	for _, m := range rePWANavHref.FindAllStringSubmatch(src, -1) {
		refs = append(refs, ConsumerRef{Consumer: "pwa-nav", Path: m[1]})
	}
	return refs, nil
}

// inventoryManifest parses the web-manifest shortcut URLs and the share-target
// action.
func inventoryManifest(root string) ([]ConsumerRef, error) {
	src, err := readFile(root, "web", "pwa", "manifest.json")
	if err != nil {
		return nil, err
	}
	var manifest struct {
		StartURL    string `json:"start_url"`
		ShareTarget struct {
			Action string `json:"action"`
		} `json:"share_target"`
		Shortcuts []struct {
			URL string `json:"url"`
		} `json:"shortcuts"`
	}
	if err := json.Unmarshal([]byte(src), &manifest); err != nil {
		return nil, fmt.Errorf("experience: parse manifest.json: %w", err)
	}
	var refs []ConsumerRef
	if strings.HasPrefix(manifest.StartURL, "/") {
		refs = append(refs, ConsumerRef{Consumer: "manifest-start-url", Path: manifest.StartURL})
	}
	if strings.HasPrefix(manifest.ShareTarget.Action, "/") {
		refs = append(refs, ConsumerRef{Consumer: "manifest-share", Path: manifest.ShareTarget.Action})
	}
	for _, sc := range manifest.Shortcuts {
		if strings.HasPrefix(sc.URL, "/") {
			refs = append(refs, ConsumerRef{Consumer: "manifest-shortcut", Path: sc.URL})
		}
	}
	return refs, nil
}

// inventoryServiceWorker parses the service-worker STATIC_ASSETS precache list.
func inventoryServiceWorker(root string) ([]ConsumerRef, error) {
	src, err := readFile(root, "web", "pwa", "sw.js")
	if err != nil {
		return nil, err
	}
	start := strings.Index(src, "STATIC_ASSETS")
	if start < 0 {
		return nil, fmt.Errorf("experience: sw.js has no STATIC_ASSETS list")
	}
	open := strings.Index(src[start:], "[")
	closeIdx := strings.Index(src[start:], "]")
	if open < 0 || closeIdx < 0 || closeIdx < open {
		return nil, fmt.Errorf("experience: sw.js STATIC_ASSETS list is malformed")
	}
	block := src[start+open : start+closeIdx]
	var refs []ConsumerRef
	for _, m := range reSWAsset.FindAllStringSubmatch(block, -1) {
		refs = append(refs, ConsumerRef{Consumer: "service-worker", Path: m[1]})
	}
	return refs, nil
}

// inventoryCatalog returns the newly-generated catalog's own bound hrefs and
// preserved current paths as consumer references (a route group / unavailable
// leaf carries "" href and no current paths, so it contributes nothing).
func inventoryCatalog(catalog ProductExperienceCatalog) []ConsumerRef {
	var refs []ConsumerRef
	for _, s := range catalog.Surfaces {
		if s.Href != "" {
			refs = append(refs, ConsumerRef{Consumer: "catalog-href", Path: s.Href})
		}
		for _, p := range s.CurrentPaths {
			refs = append(refs, ConsumerRef{Consumer: "catalog-current-path", Path: p})
		}
	}
	return refs
}

// serverRouteLiterals collects the exact route path literals registered in the
// real Chi router and the card web routes.
func serverRouteLiterals(root string) (map[string]bool, error) {
	routes := map[string]bool{}
	for _, rel := range [][]string{
		{"internal", "api", "router.go"},
		{"internal", "web", "cardrewards.go"},
	} {
		src, err := readFile(root, rel...)
		if err != nil {
			return nil, err
		}
		for _, m := range reRouteLiteral.FindAllStringSubmatch(src, -1) {
			routes[m[1]] = true
		}
		for _, m := range reRouteMethod.FindAllStringSubmatch(src, -1) {
			routes[m[1]] = true
		}
	}
	return routes, nil
}

// InventoryAll gathers every consumer reference (existing navigation authorities
// + manifest + service worker + the newly-generated catalog), sorted and
// de-duplicated for stable output.
func InventoryAll(root string, catalog ProductExperienceCatalog) ([]ConsumerRef, error) {
	var refs []ConsumerRef
	for _, fn := range []func(string) ([]ConsumerRef, error){
		inventoryServerNav, inventoryPWANav, inventoryManifest, inventoryServiceWorker,
	} {
		got, err := fn(root)
		if err != nil {
			return nil, err
		}
		refs = append(refs, got...)
	}
	refs = append(refs, inventoryCatalog(catalog)...)
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Consumer != refs[j].Consumer {
			return refs[i].Consumer < refs[j].Consumer
		}
		return refs[i].Path < refs[j].Path
	})
	return refs, nil
}

// resolves reports whether an in-app destination path resolves to a real
// registered browser route or a real PWA static asset.
func resolves(root, p string, serverRoutes map[string]bool) bool {
	if p == "" {
		return true
	}
	// Known PWA dynamic endpoints served by the router (not files).
	if p == "/pwa/" || p == "/pwa/share" {
		return true
	}
	if strings.HasPrefix(p, "/pwa/") {
		rel := strings.TrimPrefix(p, "/pwa/")
		if i := strings.IndexAny(rel, "?#"); i >= 0 {
			rel = rel[:i]
		}
		if rel == "" {
			return true
		}
		_, err := os.Stat(filepath.Join(root, "web", "pwa", filepath.FromSlash(rel)))
		return err == nil
	}
	return serverRoutes[p]
}

// StaleReferences returns the "<consumer>:<path>" entries whose destination does
// NOT resolve to a real registered route or PWA static asset. An empty result
// means every navigation, manifest, service-worker, and catalog destination is
// live — no invented or dead endpoints.
func StaleReferences(root string, catalog ProductExperienceCatalog) ([]string, error) {
	refs, err := InventoryAll(root, catalog)
	if err != nil {
		return nil, err
	}
	serverRoutes, err := serverRouteLiterals(root)
	if err != nil {
		return nil, err
	}
	var stale []string
	seen := map[string]bool{}
	for _, r := range refs {
		if resolves(root, r.Path, serverRoutes) {
			continue
		}
		key := r.Consumer + ":" + r.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		stale = append(stale, key)
	}
	sort.Strings(stale)
	return stale, nil
}
