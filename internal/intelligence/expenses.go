package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/domain"
)

// ExpenseClassifier implements the 7-level rule priority chain for expense
// classification and manages vendor normalization and suggestion generation.
type ExpenseClassifier struct {
	Pool                 *pgxpool.Pool
	IMAPExpenseLabels    map[string]string
	BusinessVendors      []string
	MinPastBusiness      int
	MinConfidence        float64
	MaxPerDigest         int
	ReclassifyBatchLimit int
	Categories           []config.ExpenseCategory
	vendorNormalizer     *VendorNormalizer
}

// NewExpenseClassifier creates a classifier from config.
func NewExpenseClassifier(pool *pgxpool.Pool, cfg *config.Config) *ExpenseClassifier {
	ec := &ExpenseClassifier{
		Pool:                 pool,
		IMAPExpenseLabels:    cfg.IMAPExpenseLabels,
		BusinessVendors:      cfg.ExpensesBusinessVendors,
		MinPastBusiness:      cfg.ExpensesSuggestionsMinPastBusiness,
		MinConfidence:        cfg.ExpensesSuggestionsMinConfidence,
		MaxPerDigest:         cfg.ExpensesSuggestionsMaxPerDigest,
		ReclassifyBatchLimit: cfg.ExpensesSuggestionsReclassifyBatchLim,
		Categories:           cfg.ExpensesCategories,
		vendorNormalizer:     NewVendorNormalizer(pool, cfg.ExpensesVendorCacheSize),
	}
	return ec
}

// Classify applies the 7-level rule priority chain to determine expense classification.
// Rules 1-4 set classification directly. Rules 5-6 only generate suggestions.
func (ec *ExpenseClassifier) Classify(expense *domain.ExpenseMetadata) string {
	// Rule 1: User correction is sticky — never overwrite
	if expense.UserCorrected && slices.Contains(expense.CorrectedFields, "classification") {
		return expense.Classification
	}

	// Rule 2: Gmail label match from IMAP_EXPENSE_LABELS config
	for _, qualifier := range expense.SourceQualifiers {
		if classification, ok := ec.IMAPExpenseLabels[qualifier]; ok {
			return classification
		}
	}

	// Rule 3: Telegram caption context — "business" or "personal" keyword
	if expense.Notes != nil {
		notesLower := strings.ToLower(*expense.Notes)
		if strings.Contains(notesLower, "business") {
			return "business"
		}
		if strings.Contains(notesLower, "personal") {
			return "personal"
		}
	}

	// Rule 4: Vendor matches business_vendors config list
	for _, bv := range ec.BusinessVendors {
		if strings.EqualFold(bv, expense.Vendor) {
			return "business"
		}
	}

	// Rules 5-6 are suggestion-only and handled by GenerateSuggestions.
	// Rule 7: No match — uncategorized
	return "uncategorized"
}

// NormalizeVendor resolves a raw vendor name to its canonical form.
func (ec *ExpenseClassifier) NormalizeVendor(ctx context.Context, vendorRaw string) (string, bool) {
	return ec.vendorNormalizer.Normalize(ctx, vendorRaw)
}

// CreateVendorAlias creates a user-sourced vendor alias from a correction.
func (ec *ExpenseClassifier) CreateVendorAlias(ctx context.Context, alias, canonical string) error {
	if ec.Pool == nil {
		return fmt.Errorf("database pool is nil")
	}
	id := ulid.Make().String()
	_, err := ec.Pool.Exec(ctx, `
		INSERT INTO vendor_aliases (id, alias, canonical, source)
		VALUES ($1, $2, $3, 'user')
		ON CONFLICT (alias) DO UPDATE SET canonical = $3, source = 'user', updated_at = NOW()
	`, id, alias, canonical)
	if err != nil {
		return fmt.Errorf("create vendor alias: %w", err)
	}
	ec.vendorNormalizer.Invalidate(alias)
	return nil
}

// SeedVendorAliases loads pre-seeded vendor aliases into the database.
func (ec *ExpenseClassifier) SeedVendorAliases(ctx context.Context) error {
	if ec.Pool == nil {
		return nil
	}
	for _, seed := range vendorSeeds {
		id := ulid.Make().String()
		_, err := ec.Pool.Exec(ctx, `
			INSERT INTO vendor_aliases (id, alias, canonical, source)
			VALUES ($1, $2, $3, 'system')
			ON CONFLICT (alias) DO NOTHING
		`, id, seed.Alias, seed.Canonical)
		if err != nil {
			slog.Warn("failed to seed vendor alias", "alias", seed.Alias, "error", err)
		}
	}
	return nil
}

// GenerateSuggestions creates business classification suggestions for
// uncategorized expenses from vendors with business history.
func (ec *ExpenseClassifier) GenerateSuggestions(ctx context.Context) (int, error) {
	if ec.Pool == nil {
		return 0, fmt.Errorf("database pool is nil")
	}

	// Find uncategorized expenses from the last 30 days
	rows, err := ec.Pool.Query(ctx, `
		SELECT a.id, metadata->'expense'->>'vendor' AS vendor
		FROM artifacts a
		WHERE metadata ? 'expense'
		AND metadata->'expense'->>'classification' = 'uncategorized'
		AND a.created_at > NOW() - INTERVAL '30 days'
		AND NOT EXISTS (
			SELECT 1 FROM expense_suggestions s
			WHERE s.artifact_id = a.id AND s.status = 'pending'
		)
		LIMIT 100
	`)
	if err != nil {
		return 0, fmt.Errorf("query uncategorized expenses: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		artifactID string
		vendor     string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.artifactID, &c.vendor); err != nil {
			continue
		}
		candidates = append(candidates, c)
	}

	created := 0
	for _, c := range candidates {
		// Check suppression
		var suppressed bool
		err := ec.Pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM expense_suggestion_suppressions
				WHERE LOWER(vendor) = LOWER($1) AND classification = 'business'
			)
		`, c.vendor).Scan(&suppressed)
		if err != nil || suppressed {
			continue
		}

		// Check business history count
		var businessCount int
		err = ec.Pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM artifacts
			WHERE metadata ? 'expense'
			AND LOWER(metadata->'expense'->>'vendor') = LOWER($1)
			AND metadata->'expense'->>'classification' = 'business'
		`, c.vendor).Scan(&businessCount)
		if err != nil || businessCount < ec.MinPastBusiness {
			continue
		}

		confidence := 0.6
		if businessCount >= 5 {
			confidence = 0.8
		} else if businessCount >= 3 {
			confidence = 0.7
		}

		evidence := fmt.Sprintf("%d previous business expenses from this vendor", businessCount)
		id := ulid.Make().String()
		_, err = ec.Pool.Exec(ctx, `
			INSERT INTO expense_suggestions (id, artifact_id, vendor, suggested_class, confidence, evidence, status)
			VALUES ($1, $2, $3, 'business', $4, $5, 'pending')
			ON CONFLICT (artifact_id, suggested_class) DO NOTHING
		`, id, c.artifactID, c.vendor, confidence, evidence)
		if err != nil {
			slog.Warn("failed to create suggestion", "artifact", c.artifactID, "error", err)
			continue
		}
		created++
	}

	return created, nil
}

// ReclassifyVendor reclassifies all uncategorized expenses from a vendor.
func (ec *ExpenseClassifier) ReclassifyVendor(ctx context.Context, vendor, classification string) (int, error) {
	if ec.Pool == nil {
		return 0, fmt.Errorf("database pool is nil")
	}

	tag, err := ec.Pool.Exec(ctx, `
		UPDATE artifacts
		SET metadata = jsonb_set(metadata, '{expense,classification}', to_jsonb($1::text))
		WHERE id IN (
			SELECT id FROM artifacts
			WHERE metadata ? 'expense'
			AND LOWER(metadata->'expense'->>'vendor') = LOWER($2)
			AND metadata->'expense'->>'classification' = 'uncategorized'
			AND (metadata->'expense'->>'user_corrected')::boolean IS NOT TRUE
			LIMIT $3
		)
	`, classification, vendor, ec.ReclassifyBatchLimit)
	if err != nil {
		return 0, fmt.Errorf("reclassify vendor: %w", err)
	}

	return int(tag.RowsAffected()), nil
}

// CategoryDisplayName returns the display name for a category slug.
func (ec *ExpenseClassifier) CategoryDisplayName(slug string) string {
	for _, c := range ec.Categories {
		if c.Slug == slug {
			return c.Display
		}
	}
	return slug
}

// VendorNormalizer resolves raw vendor names to canonical forms using an
// LRU cache backed by the vendor_aliases database table.
type VendorNormalizer struct {
	pool    *pgxpool.Pool
	mu      sync.RWMutex
	cache   map[string]string
	maxSize int
}

// NewVendorNormalizer creates a VendorNormalizer with the given cache capacity.
func NewVendorNormalizer(pool *pgxpool.Pool, maxSize int) *VendorNormalizer {
	return &VendorNormalizer{
		pool:    pool,
		cache:   make(map[string]string, maxSize),
		maxSize: maxSize,
	}
}

// Normalize resolves vendorRaw to a canonical name.
// Returns (canonical, true) if found, or ("", false) if no alias exists.
func (n *VendorNormalizer) Normalize(ctx context.Context, vendorRaw string) (string, bool) {
	key := strings.ToLower(vendorRaw)

	// Check cache first
	n.mu.RLock()
	if canonical, ok := n.cache[key]; ok {
		n.mu.RUnlock()
		if canonical == "" {
			return "", false // cached negative result
		}
		return canonical, true
	}
	n.mu.RUnlock()

	if n.pool == nil {
		return "", false
	}

	// Exact match lookup (case-insensitive)
	var canonical string
	err := n.pool.QueryRow(ctx, `
		SELECT canonical FROM vendor_aliases
		WHERE LOWER(alias) = $1
		LIMIT 1
	`, key).Scan(&canonical)

	if err == nil {
		n.put(key, canonical)
		return canonical, true
	}

	// Prefix match lookup for patterns like "SQ *" and "GOOGLE *"
	// Escape LIKE wildcards in the user input to prevent pattern injection.
	escapedKey := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(key)
	err = n.pool.QueryRow(ctx, `
		SELECT canonical FROM vendor_aliases
		WHERE alias LIKE '%*' AND $1 LIKE LOWER(REPLACE(alias, '*', '')) || '%' ESCAPE '\'
		LIMIT 1
	`, escapedKey).Scan(&canonical)

	if err == nil {
		n.put(key, canonical)
		return canonical, true
	}

	// Cache negative result
	n.put(key, "")
	return "", false
}

// Invalidate removes a cached entry.
func (n *VendorNormalizer) Invalidate(vendorRaw string) {
	n.mu.Lock()
	delete(n.cache, strings.ToLower(vendorRaw))
	n.mu.Unlock()
}

func (n *VendorNormalizer) put(key, value string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.cache) >= n.maxSize {
		// Simple eviction: clear half the cache
		i := 0
		for k := range n.cache {
			if i >= n.maxSize/2 {
				break
			}
			delete(n.cache, k)
			i++
		}
	}
	n.cache[key] = value
}
