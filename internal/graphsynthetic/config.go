package graphsynthetic

// config.go — the fail-loud configuration contract for the BUG-080-001
// SCOPE-03 product read synthetic.
//
// NO DEFAULTS, NO FALLBACKS (smackerel-no-defaults SST policy). Every
// field is REQUIRED and validated; a missing or invalid value returns a
// consolidated, actionable, value-safe error instead of silently
// substituting a guessed base URL, a guessed time window, or a guessed
// emptiness policy. An emptiness or optionality allowance must be
// NAMED explicitly — the synthetic never infers that an empty read is
// acceptable.

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/api/graphapi"
)

// CodeConfigInvalid is the value-safe configuration-failure code. Its
// message names only the field and the contract — never the bearer
// token, the resolved target host, or any secret material.
const CodeConfigInvalid = "F080-SYNTH-CONFIG-INVALID"

// edgeSourceKinds is the closed set of `source=kind:id` kinds the
// synthetic may seed its edges read from. It mirrors the graph edges
// endpoint's own allowlist, restricted to the kinds the synthetic can
// obtain a seed id for from its own fixed family sequence.
var edgeSourceKinds = []string{"topic", "person", "place"}

// edgeSeedFamilies maps an edge source kind to the LIST family whose
// first row supplies the seed id. The seed id is used ONLY to build the
// edges request URL; it is held in a local variable and never written
// into a result, a log, a span attribute, or a metric label.
var edgeSeedFamilies = map[string]graphapi.GraphRouteFamily{
	"topic":  graphapi.FamilyTopics,
	"person": graphapi.FamilyPeople,
	"place":  graphapi.FamilyPlaces,
}

// Config is the required configuration for one synthetic run.
type Config struct {
	// BaseURL is the http/https origin the synthetic reads production
	// HTTP behavior from. REQUIRED.
	BaseURL string
	// BearerToken is the real scoped session credential presented on
	// every read. REQUIRED. It is NEVER logged, emitted as a metric
	// label, placed in a span attribute, or written into any result;
	// String redacts it.
	BearerToken string
	// WindowFrom and WindowTo bound the explicit UTC time-family window
	// (inclusive start, exclusive end). REQUIRED; both must be UTC and
	// WindowTo must be strictly after WindowFrom.
	WindowFrom time.Time
	WindowTo   time.Time
	// RequestTimeout bounds every individual family read. REQUIRED and
	// must be positive, so a hung read can never stall readiness.
	RequestTimeout time.Duration
	// EdgeSourceKind names which list family seeds the edges read.
	// REQUIRED; must be one of topic, person, place.
	EdgeSourceKind string
	// AllowEmptyFamilies explicitly names the families whose zero-row
	// read is a policy-permitted true-empty. A family NOT named here
	// that reads empty is a FAILURE, never a silent pass.
	AllowEmptyFamilies []graphapi.GraphRouteFamily
	// OptionalFamilies explicitly names the families whose failure
	// degrades rather than invalidates the aggregate. A named optional
	// omission is the ONLY route to a degraded aggregate.
	OptionalFamilies []graphapi.GraphRouteFamily
	// HTTPClient is the transport used for every read. REQUIRED — the
	// caller owns transport policy (timeouts, proxy, TLS) so the
	// synthetic never fabricates one.
	HTTPClient *http.Client
}

// String renders the configuration with the bearer token redacted, so
// an accidental %v/%s of a Config can never leak the credential.
func (c Config) String() string {
	return fmt.Sprintf(
		"graphsynthetic.Config{BaseURL:%q, BearerToken:[REDACTED], WindowFrom:%s, WindowTo:%s, RequestTimeout:%s, EdgeSourceKind:%q, AllowEmptyFamilies:%v, OptionalFamilies:%v}",
		c.BaseURL, c.WindowFrom.Format(time.RFC3339), c.WindowTo.Format(time.RFC3339),
		c.RequestTimeout, c.EdgeSourceKind, c.AllowEmptyFamilies, c.OptionalFamilies,
	)
}

// Validate enforces the fail-loud contract. It joins every offending
// field into a single actionable error naming only field names and the
// contract — never the token value and never the resolved host.
func (c Config) Validate() error {
	var errs []string

	switch parsed, err := url.Parse(strings.TrimSpace(c.BaseURL)); {
	case strings.TrimSpace(c.BaseURL) == "":
		errs = append(errs, "BaseURL is required and must be a non-empty http or https origin")
	case err != nil:
		errs = append(errs, "BaseURL must be a parseable http or https origin")
	case parsed.Scheme != "http" && parsed.Scheme != "https":
		errs = append(errs, "BaseURL must use the http or https scheme")
	case parsed.Host == "":
		errs = append(errs, "BaseURL must include a host")
	}

	if strings.TrimSpace(c.BearerToken) == "" {
		errs = append(errs, "BearerToken is required; the synthetic reads with a real scoped session and never falls back to an unauthenticated read")
	}

	switch {
	case c.WindowFrom.IsZero() || c.WindowTo.IsZero():
		errs = append(errs, "WindowFrom and WindowTo are required; the time family is read with an explicit bounded UTC window")
	case c.WindowFrom.Location() != time.UTC || c.WindowTo.Location() != time.UTC:
		errs = append(errs, "WindowFrom and WindowTo must be UTC")
	case !c.WindowTo.After(c.WindowFrom):
		errs = append(errs, "WindowTo must be strictly after WindowFrom")
	}

	if c.RequestTimeout <= 0 {
		errs = append(errs, "RequestTimeout is required and must be positive so a hung read can never stall readiness")
	}

	if !slices.Contains(edgeSourceKinds, c.EdgeSourceKind) {
		errs = append(errs, fmt.Sprintf("EdgeSourceKind is required and must be one of %s", strings.Join(edgeSourceKinds, ", ")))
	}

	if c.HTTPClient == nil {
		errs = append(errs, "HTTPClient is required; the synthetic never fabricates a transport")
	}

	errs = append(errs, validateFamilySet("AllowEmptyFamilies", c.AllowEmptyFamilies)...)
	errs = append(errs, validateFamilySet("OptionalFamilies", c.OptionalFamilies)...)

	if len(errs) > 0 {
		return fmt.Errorf("[%s] invalid graph read synthetic configuration: %s", CodeConfigInvalid, strings.Join(errs, ", "))
	}
	return nil
}

// validateFamilySet rejects unknown or duplicated family names in an
// explicit allowance list, so a typo can never quietly widen the
// emptiness or optionality policy.
func validateFamilySet(field string, families []graphapi.GraphRouteFamily) []string {
	var errs []string
	canonical := graphapi.RequiredGraphFamilies()
	seen := make(map[graphapi.GraphRouteFamily]bool, len(families))
	for _, family := range families {
		if !slices.Contains(canonical, family) {
			errs = append(errs, fmt.Sprintf("%s names %q, which is not a canonical graph route family", field, family))
			continue
		}
		if seen[family] {
			errs = append(errs, fmt.Sprintf("%s names %q more than once", field, family))
			continue
		}
		seen[family] = true
	}
	return errs
}

// allowsEmpty reports whether a zero-row read of family is a
// policy-permitted true-empty.
func (c Config) allowsEmpty(family graphapi.GraphRouteFamily) bool {
	return slices.Contains(c.AllowEmptyFamilies, family)
}

// edgeSeedFamily returns the list family whose first row seeds the
// edges read for the configured EdgeSourceKind.
func (c Config) edgeSeedFamily() graphapi.GraphRouteFamily {
	return edgeSeedFamilies[c.EdgeSourceKind]
}
