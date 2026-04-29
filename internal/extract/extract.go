package extract

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	readability "github.com/go-shiori/go-readability"
)

// ContentType represents the detected type of content from a URL.
type ContentType string

const (
	ContentTypeArticle      ContentType = "article"
	ContentTypeYouTube      ContentType = "youtube"
	ContentTypeProduct      ContentType = "product"
	ContentTypeRecipe       ContentType = "recipe"
	ContentTypeGeneric      ContentType = "generic"
	ContentTypeConversation ContentType = "conversation"
	ContentTypeMediaGroup   ContentType = "media_group"
	ContentTypeVoice        ContentType = "voice"
	ContentTypeImage        ContentType = "image"
	ContentTypePDF          ContentType = "pdf"
)

// Result holds extracted content from a URL or text input.
type Result struct {
	ContentType ContentType `json:"content_type"`
	Title       string      `json:"title"`
	Author      string      `json:"author,omitempty"`
	Date        string      `json:"date,omitempty"`
	Text        string      `json:"text"`
	ContentHash string      `json:"content_hash"`
	SourceURL   string      `json:"source_url,omitempty"`
	VideoID     string      `json:"video_id,omitempty"` // YouTube only
}

var (
	youtubeRe = regexp.MustCompile(`(?:youtube\.com/watch\?v=|youtu\.be/|youtube\.com/embed/)([a-zA-Z0-9_-]{11})`)
	recipeRe  = regexp.MustCompile(`(?i)(recipe|cooking|allrecipes|epicurious|foodnetwork|seriouseats)`)
	productRe = regexp.MustCompile(`(?i)(amazon\.com/.*dp/|shopify|product|ebay\.com/itm/)`)
	imageRe   = regexp.MustCompile(`(?i)\.(jpe?g|png|gif|webp|bmp|tiff?|svg|heic)(\?|$)`)
	pdfRe     = regexp.MustCompile(`(?i)\.pdf(\?|$)`)
)

// DetectContentType determines the type of content from a URL.
func DetectContentType(rawURL string) ContentType {
	if rawURL == "" {
		return ContentTypeGeneric
	}

	if youtubeRe.MatchString(rawURL) {
		return ContentTypeYouTube
	}

	// Check file-extension-based types before domain-based heuristics
	if imageRe.MatchString(rawURL) {
		return ContentTypeImage
	}
	if pdfRe.MatchString(rawURL) {
		return ContentTypePDF
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ContentTypeGeneric
	}

	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.Path)

	if productRe.MatchString(rawURL) {
		return ContentTypeProduct
	}
	if recipeRe.MatchString(host) || recipeRe.MatchString(path) {
		return ContentTypeRecipe
	}

	return ContentTypeArticle
}

// ExtractYouTubeID returns the video ID from a YouTube URL.
func ExtractYouTubeID(rawURL string) string {
	matches := youtubeRe.FindStringSubmatch(rawURL)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// validateURLSafety blocks SSRF attempts by rejecting URLs pointing to
// private networks, loopback, link-local, or metadata endpoints.
func validateURLSafety(u *url.URL) error {
	host := u.Hostname()

	// Block non-HTTP(S) schemes
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q not allowed — only http and https", u.Scheme)
	}

	// Block common private hostnames
	lower := strings.ToLower(host)
	if lower == "localhost" || lower == "metadata.google.internal" || strings.HasSuffix(lower, ".internal") {
		return fmt.Errorf("hostname %q is a private/internal address", host)
	}

	// Resolve and check IP ranges
	ips, err := net.LookupHost(host)
	if err != nil {
		// If resolution fails, block to be safe
		return fmt.Errorf("cannot resolve hostname %q", host)
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("resolved IP %s is a private/internal address", ipStr)
		}
		// Block AWS/GCP/Azure metadata IPs
		if ipStr == "169.254.169.254" || ipStr == "169.254.170.2" {
			return fmt.Errorf("resolved IP %s is a cloud metadata endpoint", ipStr)
		}
	}

	return nil
}

// ssrfSafeTransport returns an http.Transport with a DialContext that validates
// resolved IPs at connect time, preventing DNS rebinding SSRF attacks.
func ssrfSafeTransport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("split host/port: %w", err)
			}
			ips, err := net.DefaultResolver.LookupHost(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS rebinding blocked: cannot resolve %s: %w", host, err)
			}
			for _, ipStr := range ips {
				ip := net.ParseIP(ipStr)
				if ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()) {
					return nil, fmt.Errorf("DNS rebinding blocked: %s resolved to private IP %s", host, ipStr)
				}
				if ipStr == "169.254.169.254" || ipStr == "169.254.170.2" {
					return nil, fmt.Errorf("DNS rebinding blocked: %s resolved to metadata IP %s", host, ipStr)
				}
			}
			// Connect using the first resolved IP to ensure we connect to the validated address
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
		},
	}
}

// defaultClient is the package-level HTTP client with SSRF-safe transport, reused
// across extractions to avoid per-call client/transport allocation.
var defaultClient = &http.Client{
	Timeout:   15 * time.Second,
	Transport: ssrfSafeTransport(),
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return http.ErrUseLastResponse
		}
		if err := validateURLSafety(req.URL); err != nil {
			return fmt.Errorf("redirect blocked: %w", err)
		}
		return nil
	},
}

// ExtractArticle fetches and extracts readable content from an article URL using go-readability.
func ExtractArticle(ctx context.Context, articleURL string) (*Result, error) {
	parsedURL, err := url.Parse(articleURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	// SSRF protection: reject URLs pointing to private/internal networks
	if err := validateURLSafety(parsedURL); err != nil {
		return nil, fmt.Errorf("URL blocked: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Smackerel/1.0 (content indexer)")

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, articleURL)
	}

	// Limit body size to 10MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	article, err := readability.FromReader(strings.NewReader(string(body)), parsedURL)
	if err != nil {
		slog.Warn("go-readability failed, using raw content", "url", articleURL, "error", err)
		return &Result{
			ContentType: DetectContentType(articleURL),
			Title:       parsedURL.Hostname(),
			Text:        string(body),
			ContentHash: HashContent(string(body)),
			SourceURL:   articleURL,
		}, nil
	}

	text := article.TextContent
	if text == "" {
		text = string(body)
	}

	return &Result{
		ContentType: DetectContentType(articleURL),
		Title:       article.Title,
		Author:      article.Byline,
		Text:        text,
		ContentHash: HashContent(text),
		SourceURL:   articleURL,
	}, nil
}

// ExtractText creates an extraction result from plain text input.
func ExtractText(text string) *Result {
	// Use first line as title, capped at 100 bytes (rune-safe truncation)
	title := text
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		title = text[:idx]
	}
	if len(title) > 100 {
		title = truncateTitleUTF8(title, 100)
	}

	return &Result{
		ContentType: detectPlainTextContentType(text),
		Title:       title,
		Text:        text,
		ContentHash: HashContent(text),
	}
}

func detectPlainTextContentType(text string) ContentType {
	lower := strings.ToLower(text)
	hasIngredientSection := strings.Contains(lower, "ingredients")
	hasInstructionSection := strings.Contains(lower, "instructions") || strings.Contains(lower, "directions") || strings.Contains(lower, "method")
	if hasIngredientSection && hasInstructionSection && recipeRe.MatchString(lower) {
		return ContentTypeRecipe
	}
	return ContentTypeGeneric
}

// HashContent returns the SHA-256 hex digest of the given content.
func HashContent(content string) string {
	h := sha256.New()
	h.Write([]byte(strings.TrimSpace(strings.ToLower(content))))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// truncateTitleUTF8 truncates a string to at most maxBytes bytes without
// splitting a multi-byte UTF-8 character.
func truncateTitleUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk backward from the cut point to find a valid rune boundary
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}
