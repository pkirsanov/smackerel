package list

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// ReadingAggregator creates reading list items from article/book artifacts.
type ReadingAggregator struct{}

type readingData struct {
	Domain string `json:"domain"`
	Title  string `json:"title"`
}

func (a *ReadingAggregator) Domain() string            { return "reading" }
func (a *ReadingAggregator) DefaultListType() ListType { return TypeReading }

func (a *ReadingAggregator) Aggregate(sources []AggregationSource) ([]ListItemSeed, error) {
	var seeds []ListItemSeed

	for i, src := range sources {
		// For reading lists, domain_data may be minimal. Falling back to artifact
		// metadata is intentional, but we MUST log unmarshal failures so silent
		// JSON corruption is visible to operators (no `_ = json.Unmarshal`).
		var rd readingData
		if err := json.Unmarshal(src.DomainData, &rd); err != nil {
			slog.Warn("reading aggregator: malformed domain_data, falling back to placeholder title",
				"artifact_id", src.ArtifactID, "error", err)
		}

		title := rd.Title
		if title == "" {
			title = fmt.Sprintf("Article %d", i+1)
		}

		// Estimate read time from content length if available
		contentLen := len(src.DomainData)
		readMinutes := EstimateReadTime(contentLen)

		content := title
		if readMinutes > 0 {
			content = fmt.Sprintf("%s (~%d min read)", title, readMinutes)
		}

		seeds = append(seeds, ListItemSeed{
			Content:           content,
			Category:          "reading",
			NormalizedName:    strings.ToLower(title),
			SourceArtifactIDs: []string{src.ArtifactID},
			SortOrder:         i,
		})
	}

	return seeds, nil
}

// EstimateReadTime estimates reading time in minutes from content character count.
// Based on ~200 words per minute, ~5 chars per word average.
func EstimateReadTime(charCount int) int {
	if charCount <= 0 {
		return 0
	}
	words := charCount / 5
	minutes := words / 200
	if minutes < 1 && charCount > 100 {
		return 1
	}
	return minutes
}

// CompareAggregator creates comparison items from product artifacts.
type CompareAggregator struct{}

type compareData struct {
	Domain      string        `json:"domain"`
	ProductName string        `json:"product_name"`
	Brand       string        `json:"brand"`
	Price       *comparePrice `json:"price"`
	Specs       []compareSpec `json:"specs"`
	Rating      *compareRate  `json:"rating"`
}

type comparePrice struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type compareSpec struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type compareRate struct {
	Score float64 `json:"score"`
	Max   float64 `json:"max"`
	Count int     `json:"count"`
}

func (a *CompareAggregator) Domain() string            { return "product" }
func (a *CompareAggregator) DefaultListType() ListType { return TypeComparison }

func (a *CompareAggregator) Aggregate(sources []AggregationSource) ([]ListItemSeed, error) {
	var seeds []ListItemSeed

	for i, src := range sources {
		var cd compareData
		if err := json.Unmarshal(src.DomainData, &cd); err != nil {
			// Surface malformed domain_data instead of silently dropping the
			// source. The compare aggregator previously bare-`continue`d on
			// unmarshal failure, hiding upstream extraction regressions for
			// product/comparison artifacts (parity with the recipe and reading
			// aggregators that already log the same class of failure — see
			// Gate G028 / requireNoDefaultsNoFallbacks). Behavior (skip-the-
			// bad-source) is preserved; visibility is added.
			slog.Warn("compare aggregator: skipping artifact with malformed domain_data",
				"artifact_id", src.ArtifactID, "error", err)
			continue
		}

		name := cd.ProductName
		if name == "" {
			name = fmt.Sprintf("Product %d", i+1)
		}

		// Build comparison line
		parts := []string{name}
		if cd.Brand != "" {
			parts = []string{fmt.Sprintf("%s %s", cd.Brand, name)}
		}
		if cd.Price != nil && cd.Price.Amount > 0 {
			parts = append(parts, fmt.Sprintf("$%.0f", cd.Price.Amount))
		}
		if cd.Rating != nil && cd.Rating.Score > 0 {
			parts = append(parts, fmt.Sprintf("%.1f/%g", cd.Rating.Score, cd.Rating.Max))
		}

		var pricePtr *float64
		if cd.Price != nil && cd.Price.Amount > 0 {
			pricePtr = &cd.Price.Amount
		}

		seeds = append(seeds, ListItemSeed{
			Content:           strings.Join(parts, " — "),
			Category:          "comparison",
			Quantity:          pricePtr,
			Unit:              "USD",
			NormalizedName:    strings.ToLower(name),
			SourceArtifactIDs: []string{src.ArtifactID},
			SortOrder:         i,
		})
	}

	return seeds, nil
}
