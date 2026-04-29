package recommendation

import "fmt"

// Category identifies the broad recommendation domain a provider fact or
// candidate belongs to.
type Category string

const (
	CategoryPlace   Category = "place"
	CategoryProduct Category = "product"
	CategoryDeal    Category = "deal"
	CategoryEvent   Category = "event"
	CategoryContent Category = "content"
)

// Validate returns an error when the category is outside the spec-039 set.
func (c Category) Validate() error {
	switch c {
	case CategoryPlace, CategoryProduct, CategoryDeal, CategoryEvent, CategoryContent:
		return nil
	}
	return fmt.Errorf("recommendation: invalid category %q", string(c))
}

// PrecisionPolicy controls how much location detail can leave Smackerel.
type PrecisionPolicy string

const (
	PrecisionExact        PrecisionPolicy = "exact"
	PrecisionNeighborhood PrecisionPolicy = "neighborhood"
	PrecisionCity         PrecisionPolicy = "city"
)

// Validate returns an error when the precision policy is unknown.
func (p PrecisionPolicy) Validate() error {
	switch p {
	case PrecisionExact, PrecisionNeighborhood, PrecisionCity:
		return nil
	}
	return fmt.Errorf("recommendation: invalid precision policy %q", string(p))
}
