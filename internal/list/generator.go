package list

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GenerateRequest describes what list to generate.
type GenerateRequest struct {
	ListType    ListType `json:"list_type"`
	Title       string   `json:"title"`
	ArtifactIDs []string `json:"artifact_ids,omitempty"`
	TagFilter   string   `json:"tag_filter,omitempty"`
	SearchQuery string   `json:"search_query,omitempty"`
	Domain      string   `json:"domain,omitempty"`
}

// ArtifactResolver fetches domain_data for artifacts from a backing store.
type ArtifactResolver interface {
	ResolveByIDs(ctx context.Context, ids []string) ([]AggregationSource, error)
	ResolveByTag(ctx context.Context, tag string) ([]AggregationSource, error)
	ResolveByQuery(ctx context.Context, query string) ([]AggregationSource, error)
}

// Generator resolves artifacts, selects an aggregator, and persists lists.
type Generator struct {
	Resolver    ArtifactResolver
	Store       ListStore
	Aggregators map[string]Aggregator // keyed by domain
}

// NewGenerator creates a new list generator.
func NewGenerator(resolver ArtifactResolver, store ListStore, aggregators map[string]Aggregator) *Generator {
	return &Generator{
		Resolver:    resolver,
		Store:       store,
		Aggregators: aggregators,
	}
}

// Generate creates a list from the given request.
func (g *Generator) Generate(ctx context.Context, req GenerateRequest) (*ListWithItems, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	// 1. Resolve artifacts and fetch domain_data
	sources, err := g.resolveArtifacts(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("resolve artifacts: %w", err)
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no artifacts with domain_data found for the given criteria")
	}

	// 2. Validate domain consistency — reject mixed domains
	domain, err := validateDomains(sources)
	if err != nil {
		return nil, err
	}

	// 3. Select aggregator
	agg, ok := g.Aggregators[domain]
	if !ok {
		return nil, fmt.Errorf("no aggregator registered for domain %q", domain)
	}

	// 4. Run aggregation
	seeds, err := agg.Aggregate(sources)
	if err != nil {
		return nil, fmt.Errorf("aggregation failed: %w", err)
	}

	// 5. Build list and items
	now := time.Now()
	listID := fmt.Sprintf("lst-%d", now.UnixNano())

	sourceArtifactIDs := make([]string, len(sources))
	for i, s := range sources {
		sourceArtifactIDs[i] = s.ArtifactID
	}

	listType := req.ListType
	if listType == "" {
		listType = agg.DefaultListType()
	}

	list := &List{
		ID:                listID,
		ListType:          listType,
		Title:             req.Title,
		Status:            StatusDraft,
		SourceArtifactIDs: sourceArtifactIDs,
		SourceQuery:       req.SearchQuery,
		Domain:            domain,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	items := make([]ListItem, len(seeds))
	for i, seed := range seeds {
		items[i] = ListItem{
			ID:                fmt.Sprintf("itm-%s-%d", listID[:min(8, len(listID))], i),
			ListID:            listID,
			Content:           seed.Content,
			Category:          seed.Category,
			Status:            ItemPending,
			SourceArtifactIDs: seed.SourceArtifactIDs,
			Quantity:          seed.Quantity,
			Unit:              seed.Unit,
			NormalizedName:    seed.NormalizedName,
			SortOrder:         seed.SortOrder,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
	}

	// 6. Persist via store
	if err := g.Store.CreateList(ctx, list, items); err != nil {
		return nil, fmt.Errorf("persist list: %w", err)
	}

	return &ListWithItems{List: *list, Items: items}, nil
}

// resolveArtifacts fetches domain_data for the given request, skipping artifacts without domain_data.
func (g *Generator) resolveArtifacts(ctx context.Context, req GenerateRequest) ([]AggregationSource, error) {
	var allSources []AggregationSource

	if len(req.ArtifactIDs) > 0 {
		sources, err := g.Resolver.ResolveByIDs(ctx, req.ArtifactIDs)
		if err != nil {
			return nil, fmt.Errorf("resolve by IDs: %w", err)
		}
		// Warn about artifacts that had no domain_data
		if len(sources) < len(req.ArtifactIDs) {
			slog.Warn("some artifacts lacked domain_data",
				"requested", len(req.ArtifactIDs),
				"resolved", len(sources),
			)
		}
		allSources = append(allSources, sources...)
	}

	if req.TagFilter != "" {
		sources, err := g.Resolver.ResolveByTag(ctx, req.TagFilter)
		if err != nil {
			return nil, fmt.Errorf("resolve by tag: %w", err)
		}
		allSources = append(allSources, sources...)
	}

	if req.SearchQuery != "" {
		sources, err := g.Resolver.ResolveByQuery(ctx, req.SearchQuery)
		if err != nil {
			return nil, fmt.Errorf("resolve by query: %w", err)
		}
		allSources = append(allSources, sources...)
	}

	// Deduplicate by artifact ID
	seen := make(map[string]bool)
	var unique []AggregationSource
	for _, s := range allSources {
		if !seen[s.ArtifactID] {
			seen[s.ArtifactID] = true
			unique = append(unique, s)
		}
	}

	return unique, nil
}

// domainFromData extracts the "domain" field from a domain_data JSON blob.
func domainFromData(data json.RawMessage) string {
	var d struct {
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return ""
	}
	return d.Domain
}

// validateDomains ensures all sources belong to the same domain.
// Returns the common domain or an error if sources are mixed.
func validateDomains(sources []AggregationSource) (string, error) {
	domains := make(map[string]int)
	for _, s := range sources {
		d := domainFromData(s.DomainData)
		if d != "" {
			domains[d]++
		}
	}

	if len(domains) == 0 {
		return "", fmt.Errorf("no domain information found in artifact data")
	}

	if len(domains) > 1 {
		names := make([]string, 0, len(domains))
		for d := range domains {
			names = append(names, d)
		}
		return "", fmt.Errorf("incompatible domains: artifacts span multiple domains %v; generate separate lists per domain", names)
	}

	for d := range domains {
		return d, nil
	}
	return "", fmt.Errorf("unexpected state: no domains")
}

// PostgresArtifactResolver resolves artifacts from PostgreSQL.
type PostgresArtifactResolver struct {
	Pool *pgxpool.Pool
}

// NewPostgresArtifactResolver creates a resolver backed by PostgreSQL.
func NewPostgresArtifactResolver(pool *pgxpool.Pool) *PostgresArtifactResolver {
	return &PostgresArtifactResolver{Pool: pool}
}

// ResolveByIDs fetches domain_data for the given artifact IDs.
func (r *PostgresArtifactResolver) ResolveByIDs(ctx context.Context, ids []string) ([]AggregationSource, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, domain_data FROM artifacts WHERE id = ANY($1) AND domain_data IS NOT NULL`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("query artifacts by IDs: %w", err)
	}
	defer rows.Close()
	return scanSources(rows)
}

// ResolveByTag fetches domain_data for artifacts whose topics JSONB contains the given tag.
func (r *PostgresArtifactResolver) ResolveByTag(ctx context.Context, tag string) ([]AggregationSource, error) {
	// Topics are stored as JSONB array; search for tag match in topic names
	rows, err := r.Pool.Query(ctx,
		`SELECT a.id, a.domain_data FROM artifacts a
		 WHERE a.domain_data IS NOT NULL
		   AND EXISTS (
		     SELECT 1 FROM jsonb_array_elements_text(COALESCE(a.topics, '[]'::jsonb)) t
		     WHERE lower(t) = lower($1)
		   )
		 LIMIT 50`,
		tag,
	)
	if err != nil {
		return nil, fmt.Errorf("query artifacts by tag: %w", err)
	}
	defer rows.Close()
	return scanSources(rows)
}

// ResolveByQuery fetches domain_data for artifacts matching a text search query.
func (r *PostgresArtifactResolver) ResolveByQuery(ctx context.Context, query string) ([]AggregationSource, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, domain_data FROM artifacts
		 WHERE domain_data IS NOT NULL AND title ILIKE '%' || $1 || '%'
		 ORDER BY relevance_score DESC LIMIT 50`,
		query,
	)
	if err != nil {
		return nil, fmt.Errorf("query artifacts by search: %w", err)
	}
	defer rows.Close()
	return scanSources(rows)
}

type rowScanner interface {
	Next() bool
	Scan(dest ...any) error
}

func scanSources(rows rowScanner) ([]AggregationSource, error) {
	var sources []AggregationSource
	for rows.Next() {
		var s AggregationSource
		if err := rows.Scan(&s.ArtifactID, &s.DomainData); err != nil {
			slog.Warn("failed to scan artifact domain_data", "error", err)
			continue
		}
		sources = append(sources, s)
	}
	return sources, nil
}
