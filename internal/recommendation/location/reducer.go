package location

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/smackerel/smackerel/internal/recommendation"
)

// Config carries the SST-resolved precision parameters used by the reducer.
type Config struct {
	NeighborhoodCellSystem string
	NeighborhoodCellLevel  int
}

// RawLocationRef names a local-only location reference. Raw coordinates stay
// outside the provider-facing contract.
type RawLocationRef struct {
	Ref string
}

// ReducedGeometry is the outbound shape providers may receive.
type ReducedGeometry struct {
	Precision recommendation.PrecisionPolicy
	CellID    string
	Label     string
}

// Reducer applies configured precision policy before provider lookup.
type Reducer interface {
	Reduce(ctx context.Context, ref RawLocationRef, policy recommendation.PrecisionPolicy) (ReducedGeometry, error)
}

type deterministicReducer struct {
	cfg Config
}

var rawCoordinateToken = regexp.MustCompile(`[-+]?\d+\.\d+`)

// NewReducer returns a deterministic reducer that never sends raw coordinates
// across the provider boundary.
func NewReducer(cfg Config) Reducer {
	return &deterministicReducer{cfg: cfg}
}

func (r *deterministicReducer) Reduce(_ context.Context, ref RawLocationRef, policy recommendation.PrecisionPolicy) (ReducedGeometry, error) {
	if err := policy.Validate(); err != nil {
		return ReducedGeometry{}, fmt.Errorf("location precision policy: %w", err)
	}
	if strings.TrimSpace(ref.Ref) == "" {
		return ReducedGeometry{}, fmt.Errorf("location ref is required")
	}
	if policy == recommendation.PrecisionExact {
		return ReducedGeometry{}, fmt.Errorf("exact provider location is not allowed for reactive recommendations")
	}
	if strings.TrimSpace(r.cfg.NeighborhoodCellSystem) == "" {
		return ReducedGeometry{}, fmt.Errorf("neighborhood cell system is required")
	}
	if r.cfg.NeighborhoodCellLevel <= 0 {
		return ReducedGeometry{}, fmt.Errorf("neighborhood cell level must be positive")
	}

	sanitized := sanitizeRef(ref.Ref)
	if sanitized == "" {
		sanitized = "local"
	}
	digest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(ref.Ref))))
	cellSuffix := hex.EncodeToString(digest[:])[:12]

	switch policy {
	case recommendation.PrecisionNeighborhood:
		return ReducedGeometry{
			Precision: policy,
			CellID:    fmt.Sprintf("%s-%d-%s", r.cfg.NeighborhoodCellSystem, r.cfg.NeighborhoodCellLevel, cellSuffix),
			Label:     fmt.Sprintf("%s neighborhood", sanitized),
		}, nil
	case recommendation.PrecisionCity:
		return ReducedGeometry{
			Precision: policy,
			CellID:    "city-" + cellSuffix,
			Label:     sanitized + " city area",
		}, nil
	default:
		return ReducedGeometry{}, fmt.Errorf("unsupported precision policy %q", policy)
	}
}

func sanitizeRef(ref string) string {
	value := strings.ToLower(strings.TrimSpace(ref))
	value = strings.TrimPrefix(value, "gps:")
	value = rawCoordinateToken.ReplaceAllString(value, "")
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ':' || r == '/' || r == '\\'
	})
	labels := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "-" {
			continue
		}
		labels = append(labels, part)
	}
	if len(labels) == 0 {
		return "local"
	}
	return strings.Join(labels, " ")
}
