// Package acceptance implements the BUG-102-001-owned, production-inert,
// test-only, machine-readable fault-profile registry required by JOURNEY-016 /
// SCN-102-001-12 of specs/102-target-deploy-hardening/bugs/BUG-102-001.
//
// The registry exists ONLY for disposable env=test* validate/e2e stacks. It is
// structurally inert in production: a production-posture caller can never
// resolve or activate a profile (Resolve returns ErrFaultInertInProduction for
// every stableId, even a valid one), the registry file itself must declare the
// test-only posture and forbidden production exposure, and
// AssertNoFaultControlInProductionSurface refuses any production route, config,
// request, or UI descriptor that carries a fault selector, trigger, or profile
// control.
//
// Every profile declares the closed nine-field schema BUG-102-001 owns once:
// stableId, journey, setup, teardown, parallelism, expectedRequest,
// expectedResponseOrTermination, evidence, and noFirstPartyInterception. The
// loader enforces that schema natively (strict decode plus per-field checks)
// and validates the canonical registry against its companion JSON Schema
// (config/acceptance/fault-profiles.v1.schema.json) as defense in depth.
//
// Consumers (for example BUG-073-006 SCOPE-01) reference profiles by immutable
// stableId only via Resolve. A missing or unknown stableId fails with a typed
// error and never falls back to an inline definition.
package acceptance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// Posture is the deployment posture a caller runs under. Fault profiles are
// structurally inert unless the caller declares the disposable test posture;
// a production posture can never activate a fault.
type Posture string

const (
	// PostureTest is the disposable env=test* validate/e2e posture under which
	// fault profiles may be resolved and activated.
	PostureTest Posture = "test"
	// PostureProduction is the real production build/config posture. Under this
	// posture no fault is ever resolvable or activatable.
	PostureProduction Posture = "production"
)

// Closed literals for the registry envelope. The registry is test-only and
// forbids production exposure by construction.
const (
	registryVersion  = "v1"
	registryPosture  = "test"
	registryExposure = "forbidden"
)

// closedJourneys is the closed set of product journeys a fault profile may
// target. It matches the E102-JOURNEY-* failure families BUG-102-001 owns.
var closedJourneys = map[string]bool{
	"assistant":       true,
	"session":         true,
	"search":          true,
	"digest":          true,
	"graph":           true,
	"recommendations": true,
	"cards":           true,
	"synthesis":       true,
}

// profileFieldNames is the closed nine-field schema, in declaration order,
// used for deterministic missing-field reporting.
var profileFieldNames = []string{
	"stableId",
	"journey",
	"setup",
	"teardown",
	"parallelism",
	"expectedRequest",
	"expectedResponseOrTermination",
	"evidence",
	"noFirstPartyInterception",
}

// faultControlTokens are substrings whose presence in a production route,
// configuration, request, or UI descriptor indicates a forbidden fault
// selector, trigger, or profile control. Matching is case-insensitive. The
// tokens are fault-control keywords only, never generic profile stableIds
// (for example "network" or "timeout"), so legitimate production surfaces are
// never flagged.
var faultControlTokens = []string{
	"fault-profile", "faultprofile", "fault_profile", "fault profile",
	"fault-selector", "faultselector", "fault_selector", "fault selector",
	"fault-trigger", "faulttrigger", "fault_trigger", "fault trigger",
	"fault-control", "faultcontrol", "fault_control", "fault control",
	"fault-registry", "faultregistry", "fault_registry", "fault registry",
	"inject-fault", "injectfault", "inject_fault", "inject fault",
	"activate-fault", "activatefault", "activate_fault", "activate fault",
	"trigger-fault", "triggerfault", "trigger_fault", "trigger fault",
}

// Typed errors. Callers should use errors.Is to classify a failure; every
// failure is fail-closed (the registry never silently tolerates a violation).
var (
	// ErrFaultInertInProduction is returned when a production-posture caller
	// attempts to resolve or activate any profile.
	ErrFaultInertInProduction = errors.New("fault registry: faults are production-inert and cannot be activated in a production posture")
	// ErrUnknownProfile is returned when a test-posture caller resolves a
	// stableId that is not registered. There is no silent fallback.
	ErrUnknownProfile = errors.New("fault registry: unknown profile stableId")
	// ErrUnknownPosture is returned for any posture other than test or
	// production (fail-closed: only the test posture activates a fault).
	ErrUnknownPosture = errors.New("fault registry: unknown posture")
	// ErrProductionExposureDeclared is returned when the registry envelope
	// declares anything other than the test-only, forbidden-exposure posture.
	ErrProductionExposureDeclared = errors.New("fault registry: registry declares production exposure")
	// ErrFirstPartyInterception is returned when a profile declares (or omits)
	// the no-first-party-interception assertion.
	ErrFirstPartyInterception = errors.New("fault registry: profile declares first-party interception")
	// ErrMissingField is returned when a profile omits or blanks a required
	// closed-schema field.
	ErrMissingField = errors.New("fault registry: profile missing required field")
	// ErrUnknownJourney is returned when a profile targets a journey outside
	// the closed journey set.
	ErrUnknownJourney = errors.New("fault registry: profile targets unknown journey")
	// ErrDuplicateStableID is returned when two profiles share a stableId.
	ErrDuplicateStableID = errors.New("fault registry: duplicate profile stableId")
	// ErrEmptyRegistry is returned when the registry declares zero profiles.
	ErrEmptyRegistry = errors.New("fault registry: no profiles declared")
	// ErrProductionFaultControl is returned when a production surface descriptor
	// carries a fault selector, trigger, or profile control.
	ErrProductionFaultControl = errors.New("fault registry: production surface exposes a fault control")
)

// Profile is one fault profile carrying the closed nine-field schema.
// noFirstPartyInterception is a pointer so an omitted declaration (nil) is
// distinguishable from an explicit false; both are rejected.
type Profile struct {
	StableID                      string `yaml:"stableId" json:"stableId"`
	Journey                       string `yaml:"journey" json:"journey"`
	Setup                         string `yaml:"setup" json:"setup"`
	Teardown                      string `yaml:"teardown" json:"teardown"`
	Parallelism                   string `yaml:"parallelism" json:"parallelism"`
	ExpectedRequest               string `yaml:"expectedRequest" json:"expectedRequest"`
	ExpectedResponseOrTermination string `yaml:"expectedResponseOrTermination" json:"expectedResponseOrTermination"`
	Evidence                      string `yaml:"evidence" json:"evidence"`
	NoFirstPartyInterception      *bool  `yaml:"noFirstPartyInterception" json:"noFirstPartyInterception"`
}

// Registry is the parsed, validated fault-profile registry.
type Registry struct {
	Version            string    `yaml:"version" json:"version"`
	Posture            string    `yaml:"posture" json:"posture"`
	ProductionExposure string    `yaml:"productionExposure" json:"productionExposure"`
	Profiles           []Profile `yaml:"profiles" json:"profiles"`

	byStableID map[string]Profile
}

// ParseRegistry strictly decodes and validates registry bytes against the
// closed nine-field schema: it rejects unknown fields, missing/blank required
// fields, an omitted or false no-first-party-interception assertion, an unknown
// journey, a duplicate stableId, and any registry envelope that declares
// production exposure. It performs no filesystem or JSON-Schema I/O so
// adversarial fixtures can be validated in memory. The first violation fails
// closed.
func ParseRegistry(data []byte) (*Registry, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // closed schema: any unknown field is rejected
	var reg Registry
	if err := dec.Decode(&reg); err != nil {
		return nil, fmt.Errorf("fault registry: decode: %w", err)
	}
	if err := reg.validate(); err != nil {
		return nil, err
	}
	return &reg, nil
}

func (r *Registry) validate() error {
	if r.Version != registryVersion {
		return fmt.Errorf("fault registry: unsupported version %q (want %q): %w", r.Version, registryVersion, ErrProductionExposureDeclared)
	}
	if r.Posture != registryPosture {
		return fmt.Errorf("fault registry: posture %q is not the test-only posture %q: %w", r.Posture, registryPosture, ErrProductionExposureDeclared)
	}
	if r.ProductionExposure != registryExposure {
		return fmt.Errorf("fault registry: productionExposure %q must be %q: %w", r.ProductionExposure, registryExposure, ErrProductionExposureDeclared)
	}
	if len(r.Profiles) == 0 {
		return ErrEmptyRegistry
	}
	byID := make(map[string]Profile, len(r.Profiles))
	for i := range r.Profiles {
		p := r.Profiles[i]
		if err := validateProfile(p); err != nil {
			return err
		}
		if _, dup := byID[p.StableID]; dup {
			return fmt.Errorf("fault registry: stableId %q: %w", p.StableID, ErrDuplicateStableID)
		}
		byID[p.StableID] = p
	}
	r.byStableID = byID
	return nil
}

func validateProfile(p Profile) error {
	strFields := []struct {
		name  string
		value string
	}{
		{"stableId", p.StableID},
		{"journey", p.Journey},
		{"setup", p.Setup},
		{"teardown", p.Teardown},
		{"parallelism", p.Parallelism},
		{"expectedRequest", p.ExpectedRequest},
		{"expectedResponseOrTermination", p.ExpectedResponseOrTermination},
		{"evidence", p.Evidence},
	}
	id := strings.TrimSpace(p.StableID)
	if id == "" {
		id = "<unnamed>"
	}
	for _, f := range strFields {
		if strings.TrimSpace(f.value) == "" {
			return fmt.Errorf("fault registry: profile %q field %q: %w", id, f.name, ErrMissingField)
		}
	}
	if !closedJourneys[p.Journey] {
		return fmt.Errorf("fault registry: profile %q journey %q: %w", id, p.Journey, ErrUnknownJourney)
	}
	if p.NoFirstPartyInterception == nil {
		return fmt.Errorf("fault registry: profile %q field %q: %w", id, "noFirstPartyInterception", ErrMissingField)
	}
	if !*p.NoFirstPartyInterception {
		return fmt.Errorf("fault registry: profile %q: %w", id, ErrFirstPartyInterception)
	}
	return nil
}

// Resolve looks up a profile by stableId under the caller's posture. It is the
// single consumer-lookup and activation gate:
//
//   - PostureProduction always returns ErrFaultInertInProduction, even for a
//     valid stableId, so a production build/config can never activate a fault.
//   - PostureTest returns the profile, or ErrUnknownProfile for a missing or
//     unknown stableId (no silent fallback, no inline fabrication).
//   - any other posture returns ErrUnknownPosture (fail-closed: only the test
//     posture activates a fault).
func (r *Registry) Resolve(posture Posture, stableID string) (Profile, error) {
	switch posture {
	case PostureProduction:
		return Profile{}, fmt.Errorf("fault registry: stableId %q: %w", stableID, ErrFaultInertInProduction)
	case PostureTest:
		p, ok := r.byStableID[stableID]
		if !ok {
			return Profile{}, fmt.Errorf("fault registry: stableId %q: %w", stableID, ErrUnknownProfile)
		}
		return p, nil
	default:
		return Profile{}, fmt.Errorf("fault registry: posture %q: %w", posture, ErrUnknownPosture)
	}
}

// StableIDs returns the sorted list of registered profile stableIds.
func (r *Registry) StableIDs() []string {
	ids := make([]string, 0, len(r.byStableID))
	for id := range r.byStableID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// AssertNoFaultControlInProductionSurface scans production route, configuration,
// request, and UI descriptors and refuses any that expose a fault selector,
// trigger, or profile control. Production is structurally fault-inert, so a
// clean surface returns nil and any surface carrying a fault control returns
// ErrProductionFaultControl naming the offending token.
func AssertNoFaultControlInProductionSurface(surfaces ...string) error {
	for _, s := range surfaces {
		low := strings.ToLower(s)
		for _, tok := range faultControlTokens {
			if strings.Contains(low, tok) {
				return fmt.Errorf("fault registry: production surface %q contains fault control token %q: %w", s, tok, ErrProductionFaultControl)
			}
		}
	}
	return nil
}

// CanonicalRegistryPath returns the absolute path to the committed registry
// artifact, resolved relative to this source file so it is independent of the
// process working directory.
func CanonicalRegistryPath() string {
	return filepath.Join(repoRoot(), "config", "acceptance", "fault-profiles.v1.yaml")
}

// CanonicalSchemaPath returns the absolute path to the committed JSON Schema
// companion, resolved relative to this source file.
func CanonicalSchemaPath() string {
	return filepath.Join(repoRoot(), "config", "acceptance", "fault-profiles.v1.schema.json")
}

// repoRoot resolves the repository root from this source file's location
// (internal/acceptance/fault_profile_registry.go is two directories below the
// repository root).
func repoRoot() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// LoadCanonicalRegistry reads the committed registry artifact, validates it
// against its companion JSON Schema (defense in depth), and then applies the
// native closed-schema validation. It returns the parsed registry or a typed
// error.
func LoadCanonicalRegistry() (*Registry, error) {
	regPath := CanonicalRegistryPath()
	data, err := os.ReadFile(regPath)
	if err != nil {
		return nil, fmt.Errorf("fault registry: read %s: %w", regPath, err)
	}
	if err := validateAgainstCanonicalSchema(data); err != nil {
		return nil, err
	}
	return ParseRegistry(data)
}

// validateAgainstCanonicalSchema validates registry YAML bytes against the
// committed JSON Schema by normalizing YAML to JSON and validating with
// santhosh-tekuri/jsonschema/v6 (Draft 2020-12).
func validateAgainstCanonicalSchema(yamlData []byte) error {
	schemaPath := CanonicalSchemaPath()
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("fault registry: read schema %s: %w", schemaPath, err)
	}
	parsedSchema, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		return fmt.Errorf("fault registry: schema is not valid JSON: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("fault-profiles.v1.schema.json", parsedSchema); err != nil {
		return fmt.Errorf("fault registry: add schema resource: %w", err)
	}
	schema, err := compiler.Compile("fault-profiles.v1.schema.json")
	if err != nil {
		return fmt.Errorf("fault registry: compile schema: %w", err)
	}
	var doc any
	if err := yaml.Unmarshal(yamlData, &doc); err != nil {
		return fmt.Errorf("fault registry: parse registry yaml: %w", err)
	}
	jsonBytes, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("fault registry: normalize registry to json: %w", err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonBytes))
	if err != nil {
		return fmt.Errorf("fault registry: normalize registry instance: %w", err)
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("fault registry: registry violates schema: %w", err)
	}
	return nil
}
