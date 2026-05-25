package qfdecisions

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/metrics"
)

func artifactFromEnvelope(t *testing.T, env QFDecisionPacketEnvelope) connector.RawArtifact {
	t.Helper()
	n := NewNormalizer(DefaultConnectorID, 1)
	event := validQFEvent()
	event.PacketID = env.PacketID
	event.DecisionType = env.DecisionType
	artifact, diag := n.Normalize(event, env, time.Date(2026, 5, 6, 0, 1, 0, 0, time.UTC))
	if diag != nil {
		t.Fatalf("unexpected normalizer diagnostic: %#v", diag)
	}
	if artifact == nil {
		t.Fatal("normalizer returned nil artifact")
	}
	return *artifact
}

func renderableEnvelope() QFDecisionPacketEnvelope {
	env := validQFEnvelope()
	env.PacketURLSigned = "https://qf.example.test/packets/packet-001?sig=fresh"
	env.SignatureExpiresAt = "2026-05-06T00:10:00Z"
	env.PreferredSurface = "smackerel_digest"
	env.CalibrationBadge = map[string]any{
		"label":        "QF calibrated",
		"severity":     "info",
		"summary":      "Calibration verified by QF",
		"detail":       "QF public calibration detail",
		"score":        0.982,
		"distribution": []any{0.1, 0.2, 0.7},
		"links": []any{
			map[string]any{"label": "Calibration drilldown", "url": "https://qf.example.test/calibration/packet-001", "internal_score": 0.82},
		},
	}
	env.DataProvenanceBadge = map[string]any{
		"label":            "QF provenance",
		"severity":         "warning",
		"summary":          "QF source lineage is present",
		"raw_source_count": 42,
	}
	env.QuantifiedImpact = map[string]any{
		"label":    "Impact band",
		"severity": "info",
		"summary":  "Public impact statement",
		"value":    12.5,
		"unit":     "bps",
	}
	env.ExpertAnalysisBundle = map[string]any{
		"label":              "Expert review",
		"severity":           "degraded",
		"summary":            "Human review requested",
		"model_coefficients": map[string]any{"alpha": 0.7},
	}
	return env
}

func TestRenderUnknownDecisionTypeUsesGenericCardWithoutDerivedSemantics(t *testing.T) {
	env := renderableEnvelope()
	env.DecisionType = "future_qf_shape"
	artifact := artifactFromEnvelope(t, env)

	card, err := RenderPacketCard(context.Background(), artifact, RenderOptions{
		Surface:                  SurfaceSearch,
		DeepLinkSigningSupported: true,
		Now:                      time.Date(2026, 5, 6, 0, 2, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RenderPacketCard returned error: %v", err)
	}

	if card.CardKind != CardKindGenericPacket {
		t.Fatalf("CardKind = %q, want %q", card.CardKind, CardKindGenericPacket)
	}
	if !card.UnknownDecisionType {
		t.Fatal("unknown decision type flag should be carried into the generic card")
	}
	if card.Title != env.Thesis || card.Thesis != env.Thesis || card.WhyNow != env.WhyNow {
		t.Fatalf("QF-authored content was not preserved: %#v", card)
	}
	if strings.Contains(strings.ToLower(card.DisplayLabel), "future") || strings.Contains(strings.ToLower(card.DisplayLabel), "recommendation") {
		t.Fatalf("generic card label derived semantics from unknown decision type: %q", card.DisplayLabel)
	}
	if card.PacketID != env.PacketID || card.TraceID != env.TraceID || card.ApprovalState != env.ApprovalState {
		t.Fatalf("packet identity/trust boundary metadata not preserved: %#v", card)
	}
	if card.DeepLink.URL != env.PacketURLSigned {
		t.Fatalf("DeepLink.URL = %q, want signed QF URL %q", card.DeepLink.URL, env.PacketURLSigned)
	}
	if !card.ReadOnly || card.ActionEligible {
		t.Fatalf("QF packet card must be read-only and non-actionable, got readOnly=%v actionEligible=%v", card.ReadOnly, card.ActionEligible)
	}
}

func TestTrustObjectRendererKeepsOnlyPublicFieldsForAllBadgeTypes(t *testing.T) {
	env := renderableEnvelope()
	artifact := artifactFromEnvelope(t, env)

	card, err := RenderPacketCard(context.Background(), artifact, RenderOptions{
		Surface:                  SurfaceArtifactDetail,
		DeepLinkSigningSupported: true,
		Now:                      time.Date(2026, 5, 6, 0, 2, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RenderPacketCard returned error: %v", err)
	}

	if len(card.TrustObjects) != 4 {
		t.Fatalf("TrustObjects length = %d, want 4: %#v", len(card.TrustObjects), card.TrustObjects)
	}
	for _, trust := range card.TrustObjects {
		if trust.Label == "" || trust.Severity == "" || trust.Summary == "" {
			t.Fatalf("public trust fields not preserved for %s: %#v", trust.Kind, trust)
		}
		rendered := trust.Label + trust.Severity + trust.Summary + trust.Detail
		if strings.Contains(rendered, "0.982") || strings.Contains(rendered, "42") || strings.Contains(rendered, "bps") || strings.Contains(rendered, "alpha") {
			t.Fatalf("numeric or internal QF fields leaked into trust render output for %s: %#v", trust.Kind, trust)
		}
		for _, link := range trust.Links {
			if link.Label == "" || link.URL == "" {
				t.Fatalf("trust link should keep only public label/url, got %#v", link)
			}
			if strings.Contains(link.Label+link.URL, "0.82") || strings.Contains(link.Label+link.URL, "internal_score") {
				t.Fatalf("trust link leaked internal fields: %#v", link)
			}
		}
	}
}

func TestTrustObjectMissingRequiredFieldFallsBackAndEmitsMetric(t *testing.T) {
	metrics.QFTrustObjectRenderFailures.Reset()
	env := renderableEnvelope()
	delete(env.DataProvenanceBadge, "severity")
	artifact := artifactFromEnvelope(t, env)

	card, err := RenderPacketCard(context.Background(), artifact, RenderOptions{
		Surface:                  SurfaceDigest,
		DeepLinkSigningSupported: true,
		Now:                      time.Date(2026, 5, 6, 0, 2, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RenderPacketCard returned error: %v", err)
	}

	if card.CardKind != CardKindGenericPacket || card.FallbackReason != TrustFallbackMissingRequiredField {
		t.Fatalf("card should fall back through generic packet path, got kind=%q reason=%q", card.CardKind, card.FallbackReason)
	}
	if len(card.TrustObjects) != 0 {
		t.Fatalf("incomplete trust objects must not render, got %#v", card.TrustObjects)
	}
	if card.PacketID != env.PacketID || card.TraceID != env.TraceID || card.ApprovalState != env.ApprovalState || card.DeepLink.URL != env.PacketURLSigned {
		t.Fatalf("fallback lost packet identity/trust boundary/deep link: %#v", card)
	}
	got := testutil.ToFloat64(metrics.QFTrustObjectRenderFailures.WithLabelValues(TrustFallbackMissingRequiredField))
	if got != 1 {
		t.Fatalf("trust object render failure metric = %v, want 1", got)
	}
}

func TestSignedDeepLinkSelectionUsesSignedRefetchesExpiredAndFallsBackOnlyWhenUnsupported(t *testing.T) {
	fixedNow := time.Date(2026, 5, 6, 0, 2, 0, 0, time.UTC)

	t.Run("signed_used", func(t *testing.T) {
		metrics.QFDeepLinkRenderTotal.Reset()
		env := renderableEnvelope()
		artifact := artifactFromEnvelope(t, env)

		card, err := RenderPacketCard(context.Background(), artifact, RenderOptions{Surface: SurfaceWeb, DeepLinkSigningSupported: true, Now: fixedNow})
		if err != nil {
			t.Fatalf("RenderPacketCard returned error: %v", err)
		}
		if card.DeepLink.URL != env.PacketURLSigned || card.DeepLink.Status != DeepLinkStatusSignedUsed {
			t.Fatalf("deep link = %#v, want signed_used %q", card.DeepLink, env.PacketURLSigned)
		}
		got := testutil.ToFloat64(metrics.QFDeepLinkRenderTotal.WithLabelValues(SurfaceWeb, DeepLinkStatusSignedUsed))
		if got != 1 {
			t.Fatalf("signed_used metric = %v, want 1", got)
		}
	})

	t.Run("expired_refetches_fresh_signed", func(t *testing.T) {
		metrics.QFDeepLinkRenderTotal.Reset()
		env := renderableEnvelope()
		env.PacketURLSigned = "https://qf.example.test/packets/packet-001?sig=expired"
		env.SignatureExpiresAt = "2026-05-06T00:00:01Z"
		artifact := artifactFromEnvelope(t, env)
		refetched := renderableEnvelope()
		refetched.PacketURLSigned = "https://qf.example.test/packets/packet-001?sig=refetched"
		refetched.SignatureExpiresAt = "2026-05-06T00:20:00Z"
		called := 0

		card, err := RenderPacketCard(context.Background(), artifact, RenderOptions{
			Surface:                  SurfaceTelegram,
			DeepLinkSigningSupported: true,
			Now:                      fixedNow,
			FetchPacket: func(_ context.Context, packetID string) (QFDecisionPacketEnvelope, error) {
				called++
				if packetID != env.PacketID {
					t.Fatalf("FetchPacket packetID = %q, want %q", packetID, env.PacketID)
				}
				return refetched, nil
			},
		})
		if err != nil {
			t.Fatalf("RenderPacketCard returned error: %v", err)
		}
		if called != 1 {
			t.Fatalf("FetchPacket called %d times, want 1", called)
		}
		if card.DeepLink.URL != refetched.PacketURLSigned || card.DeepLink.Status != DeepLinkStatusSignedUsed {
			t.Fatalf("deep link = %#v, want refetched signed URL", card.DeepLink)
		}
		got := testutil.ToFloat64(metrics.QFDeepLinkRenderTotal.WithLabelValues(SurfaceTelegram, DeepLinkStatusSignedUsed))
		if got != 1 {
			t.Fatalf("refetched signed_used metric = %v, want 1", got)
		}
	})

	t.Run("expired_refetch_failure_falls_back_unsigned", func(t *testing.T) {
		metrics.QFDeepLinkRenderTotal.Reset()
		env := renderableEnvelope()
		env.PacketURLSigned = "https://qf.example.test/packets/packet-001?sig=expired"
		env.SignatureExpiresAt = "2026-05-06T00:00:01Z"
		artifact := artifactFromEnvelope(t, env)

		card, err := RenderPacketCard(context.Background(), artifact, RenderOptions{
			Surface:                  SurfaceSearch,
			DeepLinkSigningSupported: true,
			Now:                      fixedNow,
			FetchPacket: func(context.Context, string) (QFDecisionPacketEnvelope, error) {
				return QFDecisionPacketEnvelope{}, errors.New("qf unavailable")
			},
		})
		if err != nil {
			t.Fatalf("RenderPacketCard returned error: %v", err)
		}
		if card.DeepLink.URL != env.DeepLink || card.DeepLink.Status != DeepLinkStatusSignedExpiredFallbackUnsigned {
			t.Fatalf("deep link = %#v, want expired fallback to unsigned %q", card.DeepLink, env.DeepLink)
		}
		got := testutil.ToFloat64(metrics.QFDeepLinkRenderTotal.WithLabelValues(SurfaceSearch, DeepLinkStatusSignedExpiredFallbackUnsigned))
		if got != 1 {
			t.Fatalf("expired fallback metric = %v, want 1", got)
		}
	})

	t.Run("unsigned_only_when_packet_has_no_signed_url", func(t *testing.T) {
		metrics.QFDeepLinkRenderTotal.Reset()
		env := renderableEnvelope()
		env.PacketURLSigned = ""
		env.SignatureExpiresAt = ""
		artifact := artifactFromEnvelope(t, env)
		called := false

		card, err := RenderPacketCard(context.Background(), artifact, RenderOptions{
			Surface:                  SurfaceSearch,
			DeepLinkSigningSupported: true,
			Now:                      fixedNow,
			FetchPacket: func(context.Context, string) (QFDecisionPacketEnvelope, error) {
				called = true
				return QFDecisionPacketEnvelope{}, nil
			},
		})
		if err != nil {
			t.Fatalf("RenderPacketCard returned error: %v", err)
		}
		if called {
			t.Fatal("FetchPacket must not be called when the packet has no signed URL to refresh")
		}
		if card.DeepLink.URL != env.DeepLink || card.DeepLink.Status != DeepLinkStatusUnsignedOnly {
			t.Fatalf("deep link = %#v, want unsigned_only %q", card.DeepLink, env.DeepLink)
		}
		got := testutil.ToFloat64(metrics.QFDeepLinkRenderTotal.WithLabelValues(SurfaceSearch, DeepLinkStatusUnsignedOnly))
		if got != 1 {
			t.Fatalf("unsigned_only metric = %v, want 1", got)
		}
	})

	t.Run("unsigned_only_when_capability_disables_signing", func(t *testing.T) {
		metrics.QFDeepLinkRenderTotal.Reset()
		env := renderableEnvelope()
		artifact := artifactFromEnvelope(t, env)
		called := false

		card, err := RenderPacketCard(context.Background(), artifact, RenderOptions{
			Surface:                  SurfaceDigest,
			DeepLinkSigningSupported: false,
			Now:                      fixedNow,
			FetchPacket: func(context.Context, string) (QFDecisionPacketEnvelope, error) {
				called = true
				return QFDecisionPacketEnvelope{}, nil
			},
		})
		if err != nil {
			t.Fatalf("RenderPacketCard returned error: %v", err)
		}
		if called {
			t.Fatal("FetchPacket must not be called when signing capability is disabled")
		}
		if card.DeepLink.URL != env.DeepLink || card.DeepLink.Status != DeepLinkStatusUnsignedOnly {
			t.Fatalf("deep link = %#v, want unsigned_only %q", card.DeepLink, env.DeepLink)
		}
		got := testutil.ToFloat64(metrics.QFDeepLinkRenderTotal.WithLabelValues(SurfaceDigest, DeepLinkStatusUnsignedOnly))
		if got != 1 {
			t.Fatalf("unsigned_only metric = %v, want 1", got)
		}
	})
}

func TestRenderPacketCardRecordsRenderAndTotalFreshness(t *testing.T) {
	metrics.QFFreshnessP95Seconds.Reset()
	resetGlobalFreshnessForTest()
	env := renderableEnvelope()
	env.CreatedAt = "2026-05-06T00:00:00Z"
	env.UpdatedAt = "2026-05-06T00:01:00Z"
	artifact := artifactFromEnvelope(t, env)
	observedAt := time.Date(2026, 5, 6, 0, 2, 0, 0, time.UTC)

	_, err := RenderPacketCard(context.Background(), artifact, RenderOptions{
		Surface:                  SurfaceWeb,
		DeepLinkSigningSupported: true,
		Now:                      observedAt,
	})
	if err != nil {
		t.Fatalf("RenderPacketCard returned error: %v", err)
	}

	renderP95 := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageRender))
	if renderP95 != 60 {
		t.Fatalf("render freshness p95 = %v, want 60 seconds from artifact capture to render", renderP95)
	}
	totalP95 := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageTotal))
	if totalP95 != 120 {
		t.Fatalf("total freshness p95 = %v, want 120 seconds from QF create to render", totalP95)
	}
}

func TestPreferredSurfaceRoutingBranchesDoNotMutateTrustOrActionState(t *testing.T) {
	fixedNow := time.Date(2026, 5, 6, 0, 2, 0, 0, time.UTC)
	base := renderableEnvelope()
	base.PreferredSurface = ""
	baselineArtifact := artifactFromEnvelope(t, base)
	baseline, err := RenderPacketCard(context.Background(), baselineArtifact, RenderOptions{Surface: SurfaceWeb, DeepLinkSigningSupported: true, PreferredSurfaceHintSupported: true, Now: fixedNow})
	if err != nil {
		t.Fatalf("baseline render failed: %v", err)
	}

	for _, preferredSurface := range []string{"smackerel_digest", "smackerel_telegram", "qf_dashboard", "any", ""} {
		env := renderableEnvelope()
		env.PreferredSurface = preferredSurface
		artifact := artifactFromEnvelope(t, env)
		card, err := RenderPacketCard(context.Background(), artifact, RenderOptions{Surface: SurfaceWeb, DeepLinkSigningSupported: true, PreferredSurfaceHintSupported: true, Now: fixedNow})
		if err != nil {
			t.Fatalf("render failed for preferred_surface=%q: %v", preferredSurface, err)
		}

		if card.Thesis != baseline.Thesis || card.WhyNow != baseline.WhyNow || card.ApprovalState != baseline.ApprovalState || card.ActionEligible != baseline.ActionEligible || card.DeepLink.URL != baseline.DeepLink.URL {
			t.Fatalf("preferred_surface=%q mutated content/action/link: got %#v baseline %#v", preferredSurface, card, baseline)
		}
		if !reflect.DeepEqual(card.TrustObjects, baseline.TrustObjects) {
			t.Fatalf("preferred_surface=%q mutated trust objects: got %#v baseline %#v", preferredSurface, card.TrustObjects, baseline.TrustObjects)
		}

		switch preferredSurface {
		case "smackerel_digest":
			if !card.Placement.IncludeInDigest || card.Placement.QueueTelegram || card.Placement.PrimarySurface != PreferredSurfaceSmackerelDigest {
				t.Fatalf("smackerel_digest placement = %#v", card.Placement)
			}
		case "smackerel_telegram":
			if !card.Placement.QueueTelegram || card.Placement.IncludeInDigest || card.Placement.PrimarySurface != PreferredSurfaceSmackerelTelegram {
				t.Fatalf("smackerel_telegram placement = %#v", card.Placement)
			}
		case "qf_dashboard":
			if card.Placement.IncludeInDigest || card.Placement.QueueTelegram || !card.Placement.ShowInQFDashboard {
				t.Fatalf("qf_dashboard placement = %#v", card.Placement)
			}
		case "any":
			if !card.Placement.IncludeInDigest || !card.Placement.QueueTelegram || card.Placement.PrimarySurface != PreferredSurfaceAny {
				t.Fatalf("any placement = %#v", card.Placement)
			}
		case "":
			if card.Placement.PrimarySurface != PreferredSurfaceQFDashboard {
				t.Fatalf("missing preferred_surface should use recommendation default qf_dashboard, got %#v", card.Placement)
			}
		}
	}
}
