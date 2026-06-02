// Package config — Spec 076 SCOPE-1: assistant.annotation.classifier.* SST.
//
// Foundation seam that the spec 066 SCOPE-5 cut-over (annotation
// classifier swap, `interactionMap` deletion, warm-cache consistency)
// will consume in spec 076 SCOPE-4. Spec 066 declared these keys
// design-only; spec 076 SCOPE-1 ships them through the SST pipeline
// so every later scope can load them via fail-loud LookupEnv.
//
// No in-source defaults (Gate G028, smackerel-no-defaults): every
// env var MUST be present at load time; deep validation rejects
// empty / out-of-range values unconditionally.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// AnnotationClassifierConfig is the SST surface for spec 076 SCOPE-1
// foundation keys named in the SCN-076-F02 fail-loud invariant:
//
//   - assistant.annotation.classifier.confidence_floor
//   - assistant.annotation.classifier.warm_cache_enabled
type AnnotationClassifierConfig struct {
	// ConfidenceFloor is the minimum LLM classification confidence
	// the annotation cut-over (spec 076 SCOPE-4) will accept before
	// routing to spec 061 disambiguation. MUST be in [0,1].
	ConfidenceFloor float64
	// WarmCacheEnabled gates the bounded exact-token latency cache.
	// MUST be strict bool ("true"|"false").
	WarmCacheEnabled bool
}

// LoadAnnotationClassifier reads every
// ASSISTANT_ANNOTATION_CLASSIFIER_* env var and returns a populated
// AnnotationClassifierConfig plus Validate() result. Missing env vars
// (LookupEnv == false) are a fail-loud [F076-SST-MISSING] error.
// Empty/invalid values are routed through Validate() which produces
// [F076-SST-INVALID].
func LoadAnnotationClassifier() (AnnotationClassifierConfig, error) {
	var cfg AnnotationClassifierConfig
	var errs []string

	if v, ok := os.LookupEnv("ASSISTANT_ANNOTATION_CLASSIFIER_CONFIDENCE_FLOOR"); !ok {
		errs = append(errs, "ASSISTANT_ANNOTATION_CLASSIFIER_CONFIDENCE_FLOOR (env var not set)")
	} else if v == "" {
		errs = append(errs, "ASSISTANT_ANNOTATION_CLASSIFIER_CONFIDENCE_FLOOR (empty)")
	} else if f, err := strconv.ParseFloat(v, 64); err != nil {
		errs = append(errs, fmt.Sprintf("ASSISTANT_ANNOTATION_CLASSIFIER_CONFIDENCE_FLOOR (must be a float, got %q)", v))
	} else {
		cfg.ConfidenceFloor = f
	}

	if v, ok := os.LookupEnv("ASSISTANT_ANNOTATION_CLASSIFIER_WARM_CACHE_ENABLED"); !ok {
		errs = append(errs, "ASSISTANT_ANNOTATION_CLASSIFIER_WARM_CACHE_ENABLED (env var not set)")
	} else {
		switch v {
		case "true":
			cfg.WarmCacheEnabled = true
		case "false":
			cfg.WarmCacheEnabled = false
		default:
			errs = append(errs, fmt.Sprintf("ASSISTANT_ANNOTATION_CLASSIFIER_WARM_CACHE_ENABLED (must be exactly %q or %q, got %q)", "true", "false", v))
		}
	}

	if len(errs) > 0 {
		return AnnotationClassifierConfig{}, fmt.Errorf("[F076-SST-MISSING] missing or invalid required assistant.annotation.classifier configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return AnnotationClassifierConfig{}, err
	}
	return cfg, nil
}

// Validate enforces the range invariants documented on each field.
func (c *AnnotationClassifierConfig) Validate() error {
	var errs []string
	if c.ConfidenceFloor < 0 || c.ConfidenceFloor > 1 {
		errs = append(errs, fmt.Sprintf("assistant.annotation.classifier.confidence_floor (must be in [0,1], got %g)", c.ConfidenceFloor))
	}
	if len(errs) > 0 {
		return fmt.Errorf("[F076-SST-INVALID] invalid assistant.annotation.classifier configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}
