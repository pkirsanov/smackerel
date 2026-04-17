// Package list provides the actionable list model, aggregators, store,
// and generator for creating lists from domain-extracted artifact data.
package list

import (
	"encoding/json"
	"time"
)

// ListType identifies the kind of list.
type ListType string

const (
	TypeShopping   ListType = "shopping"
	TypeReading    ListType = "reading"
	TypeComparison ListType = "comparison"
	TypePacking    ListType = "packing"
	TypeChecklist  ListType = "checklist"
	TypeCustom     ListType = "custom"
)

// ListStatus identifies the lifecycle state of a list.
type ListStatus string

const (
	StatusDraft     ListStatus = "draft"
	StatusActive    ListStatus = "active"
	StatusCompleted ListStatus = "completed"
	StatusArchived  ListStatus = "archived"
)

// ItemStatus identifies the state of a list item.
type ItemStatus string

const (
	ItemPending     ItemStatus = "pending"
	ItemDone        ItemStatus = "done"
	ItemSkipped     ItemStatus = "skipped"
	ItemSubstituted ItemStatus = "substituted"
)

// List is an aggregate container for list items.
type List struct {
	ID                string     `json:"id"`
	ListType          ListType   `json:"list_type"`
	Title             string     `json:"title"`
	Status            ListStatus `json:"status"`
	SourceArtifactIDs []string   `json:"source_artifact_ids"`
	SourceQuery       string     `json:"source_query,omitempty"`
	Domain            string     `json:"domain,omitempty"`
	TotalItems        int        `json:"total_items"`
	CheckedItems      int        `json:"checked_items"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
}

// ListItem is an individual trackable entry in a list.
type ListItem struct {
	ID                string     `json:"id"`
	ListID            string     `json:"list_id"`
	Content           string     `json:"content"`
	Category          string     `json:"category,omitempty"`
	Status            ItemStatus `json:"status"`
	Substitution      string     `json:"substitution,omitempty"`
	SourceArtifactIDs []string   `json:"source_artifact_ids"`
	IsManual          bool       `json:"is_manual"`
	Quantity          *float64   `json:"quantity,omitempty"`
	Unit              string     `json:"unit,omitempty"`
	NormalizedName    string     `json:"normalized_name,omitempty"`
	SortOrder         int        `json:"sort_order"`
	CheckedAt         *time.Time `json:"checked_at,omitempty"`
	Notes             string     `json:"notes,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// ListWithItems combines a list with its items.
type ListWithItems struct {
	List  List       `json:"list"`
	Items []ListItem `json:"items"`
}

// AggregationSource represents domain_data from one artifact for aggregation.
type AggregationSource struct {
	ArtifactID string          `json:"artifact_id"`
	DomainData json.RawMessage `json:"domain_data"`
}

// ListItemSeed is the output of an aggregator before persistence.
type ListItemSeed struct {
	Content           string   `json:"content"`
	Category          string   `json:"category"`
	Quantity          *float64 `json:"quantity,omitempty"`
	Unit              string   `json:"unit"`
	NormalizedName    string   `json:"normalized_name"`
	SourceArtifactIDs []string `json:"source_artifact_ids"`
	SortOrder         int      `json:"sort_order"`
}

// Aggregator transforms domain_data from multiple artifacts into list items.
type Aggregator interface {
	// Aggregate takes domain_data from multiple artifacts and returns list items.
	Aggregate(sources []AggregationSource) ([]ListItemSeed, error)
	// Domain returns the domain this aggregator handles.
	Domain() string
	// ListType returns the default list type for this domain.
	DefaultListType() ListType
}
