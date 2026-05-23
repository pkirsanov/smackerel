package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/metrics"
)

// NewRouter creates the Chi router with all routes and middleware.
func NewRouter(deps *Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(structuredLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/ping"))
	r.Use(securityHeadersMiddleware)

	// CORS — configured via SST (CORSAllowedOrigins from smackerel.yaml).
	// Default: no origins allowed (same-origin only). Set cors.allowed_origins
	// in smackerel.yaml to enable cross-origin access for web clients.
	if len(deps.CORSAllowedOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   deps.CORSAllowedOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
			ExposedHeaders:   []string{"Link"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	// Prometheus metrics endpoint — unauthenticated (standard scrape pattern)
	r.Handle("/metrics", metrics.Handler())

	// Readiness probe — lightweight check for orchestrators (k8s, Docker HEALTHCHECK).
	// Only verifies DB connectivity; /api/health is the full liveness check.
	r.Get("/readyz", deps.ReadyzHandler)

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.Throttle(100))

		// Health endpoint — no auth required (monitoring)
		r.Get("/health", deps.HealthHandler)

		// Authenticated API routes
		r.Group(func(r chi.Router) {
			r.Use(deps.bearerAuthMiddleware)
			r.Post("/capture", deps.CaptureHandler)
			r.Post("/search", deps.SearchHandler)
			r.Get("/digest", deps.DigestHandler)
			r.Get("/recent", deps.RecentHandler)
			r.Get("/artifact/{id}", deps.ArtifactDetailHandler)
			r.Get("/artifacts/{id}/domain", deps.DomainDataHandler)
			r.Get("/export", deps.ExportHandler)

			// Context enrichment endpoint (GuestHost connector)
			if deps.ContextHandler != nil {
				r.Post("/context-for", deps.ContextHandler.HandleContextFor)
			}

			// Bookmark import endpoint (Phase 2)
			r.Post("/bookmarks/import", deps.BookmarkImportHandler)

			// Annotation endpoints (spec 027)
			if deps.AnnotationHandlers != nil {
				r.Route("/artifacts/{id}/annotations", func(r chi.Router) {
					r.Post("/", deps.AnnotationHandlers.CreateAnnotation)
					r.Get("/", deps.AnnotationHandlers.GetAnnotations)
					r.Get("/summary", deps.AnnotationHandlers.GetAnnotationSummary)
				})
				r.Delete("/artifacts/{id}/tags/{tag}", deps.AnnotationHandlers.DeleteTag)
				// Internal Telegram message-artifact mapping (spec 027, scope 5)
				r.Post("/internal/telegram-message-artifact", deps.AnnotationHandlers.RecordTelegramMessageArtifact)
				r.Get("/internal/telegram-message-artifact", deps.AnnotationHandlers.ResolveTelegramMessageArtifact)
			}

			// Knowledge layer endpoints (Scope 3)
			r.Route("/knowledge", func(r chi.Router) {
				r.Get("/concepts", deps.KnowledgeConceptsHandler)
				r.Get("/concepts/{id}", deps.KnowledgeConceptDetailHandler)
				r.Get("/entities", deps.KnowledgeEntitiesHandler)
				r.Get("/entities/{id}", deps.KnowledgeEntityDetailHandler)
				r.Get("/lint", deps.KnowledgeLintHandler)
				r.Get("/stats", deps.KnowledgeStatsHandler)
			})

			// Phase 5 intelligence endpoints (R-501..R-508)
			if deps.IntelligenceEngine != nil {
				r.Get("/expertise", ExpertiseHandler(deps.IntelligenceEngine))
				r.Get("/learning-paths", LearningPathsHandler(deps.IntelligenceEngine))
				r.Get("/subscriptions", SubscriptionsHandler(deps.IntelligenceEngine))
				r.Get("/serendipity", SerendipityHandler(deps.IntelligenceEngine))
				r.Get("/content-fuel", ContentFuelHandler(deps.IntelligenceEngine))
				r.Get("/quick-references", QuickReferencesHandler(deps.IntelligenceEngine))
				r.Get("/monthly-report", MonthlyReportHandler(deps.IntelligenceEngine))
				r.Get("/seasonal-patterns", SeasonalPatternsHandler(deps.IntelligenceEngine))
			}

			// Actionable list endpoints (spec 028)
			if deps.ListHandlers != nil {
				r.Route("/lists", func(r chi.Router) {
					r.Post("/", deps.ListHandlers.CreateListHandler)
					r.Get("/", deps.ListHandlers.ListListsHandler)
					r.Get("/{id}", deps.ListHandlers.GetListHandler)
					r.Patch("/{id}", deps.ListHandlers.UpdateListHandler)
					r.Delete("/{id}", deps.ListHandlers.ArchiveListHandler)
					r.Post("/{id}/items", deps.ListHandlers.AddItemHandler)
					r.Post("/{id}/items/{itemId}/check", deps.ListHandlers.CheckItemHandler)
					r.Delete("/{id}/items/{itemId}", deps.ListHandlers.RemoveItemHandler)
					r.Post("/{id}/complete", deps.ListHandlers.CompleteListHandler)
				})
			}

			// OAuth status requires authentication (token-bearing callers)
			if deps.OAuthHandler != nil {
				r.Get("/auth/status", deps.OAuthHandler.StatusHandler)
			}

			// Expense tracking endpoints (spec 034)
			if deps.ExpenseHandler != nil {
				deps.ExpenseHandler.RegisterRoutes(r)
			}

			// Meal planning endpoints (spec 036)
			if deps.MealPlanHandler != nil {
				deps.MealPlanHandler.RegisterRoutes(r)
			}

			// Recommendation endpoints (spec 039)
			if deps.RecommendationHandlers != nil {
				r.Route("/recommendations", func(r chi.Router) {
					r.Post("/requests", deps.RecommendationHandlers.CreateRequest)
					r.Get("/requests/{id}", deps.RecommendationHandlers.GetRequest)
					r.Get("/preferences", deps.RecommendationHandlers.ListPreferences)
					r.Post("/preferences/{key}/corrections", deps.RecommendationHandlers.CreatePreferenceCorrection)
					r.Delete("/preferences/{key}/corrections/{correctionID}", deps.RecommendationHandlers.RevokePreferenceCorrection)
					r.Get("/providers", deps.RecommendationHandlers.ListProviders)
					r.Get("/{id}/why", deps.RecommendationHandlers.GetWhy)
					r.Post("/{id}/feedback", deps.RecommendationHandlers.RecordFeedback)
					r.Get("/{id}", deps.RecommendationHandlers.GetRecommendation)
					if deps.RecommendationWatchHandlers != nil {
						r.Get("/watches", deps.RecommendationWatchHandlers.ListWatches)
						r.Post("/watches", deps.RecommendationWatchHandlers.CreateWatch)
						r.Get("/watches/{id}", deps.RecommendationWatchHandlers.GetWatch)
						r.Put("/watches/{id}", deps.RecommendationWatchHandlers.UpdateWatch)
						r.Delete("/watches/{id}", deps.RecommendationWatchHandlers.DeleteWatch)
						r.Post("/watches/{id}/pause", deps.RecommendationWatchHandlers.PauseWatch)
						r.Post("/watches/{id}/resume", deps.RecommendationWatchHandlers.ResumeWatch)
						r.Post("/watches/{id}/silence", deps.RecommendationWatchHandlers.SilenceWatch)
						r.Post("/watches/{id}/trigger", deps.RecommendationWatchHandlers.TriggerWatch)
					}
				})
			}

			if deps.QFEvidenceHandlers != nil {
				r.Route("/qf/evidence-bundles", func(r chi.Router) {
					r.Post("/", deps.QFEvidenceHandlers.CreateExport)
					r.Get("/{exportID}", deps.QFEvidenceHandlers.GetExport)
					r.Delete("/{exportID}", deps.QFEvidenceHandlers.RevokeExport)
				})
			}

			// Spec 041 Scope 7 — QF Companion personal-context read API
			// host. The route is mounted INSIDE the bearer-auth gated
			// group; consent-token validation, capability gating, and
			// per-token rate limiting happen inside the handler.
			if deps.PersonalContextHandlers != nil {
				r.Get("/private/qf/v1/personal-context", deps.PersonalContextHandlers.Read)
			}

			if deps.NotificationHandlers != nil {
				r.Route("/notifications", func(r chi.Router) {
					r.Get("/status", deps.NotificationHandlers.Status)
					r.Get("/sources", deps.NotificationHandlers.ListSources)
					r.Get("/events", deps.NotificationHandlers.ListEvents)
					r.Get("/events/{event_id}", deps.NotificationHandlers.GetEvent)
					r.Post("/manual-ingest", deps.NotificationHandlers.ManualIngest)
					r.Get("/incidents", deps.NotificationHandlers.ListIncidents)
					r.Get("/incidents/{incident_id}", deps.NotificationHandlers.GetIncident)
					r.Post("/incidents/{incident_id}/snooze", deps.NotificationHandlers.SnoozeIncident)
					r.Get("/suppressions", deps.NotificationHandlers.ListSuppressions)
					r.Get("/quiet-windows", deps.NotificationHandlers.ListQuietWindows)
					r.Get("/summary", deps.NotificationHandlers.Summary)
					r.Get("/outputs", deps.NotificationHandlers.ListOutputs)
					r.Get("/approvals/{approval_id}", deps.NotificationHandlers.GetApproval)
					r.Post("/approvals/{approval_id}/decisions", deps.NotificationHandlers.RecordApprovalDecision)
				})
			}
		})
	})

	// OAuth routes — no Bearer auth (browser redirect flow)
	// Both start and callback are rate-limited to prevent abuse (SEC-SWEEP-001).
	if deps.OAuthHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(httprate.LimitByIP(10, 1*time.Minute))
			r.Get("/auth/{provider}/start", deps.OAuthHandler.StartHandler)
			r.Get("/auth/{provider}/callback", deps.OAuthHandler.CallbackHandler)
		})
	}

	// Spec 044 Scope 03 — PWA per-user session foundation.
	// POST /v1/web/login converts a per-user PASETO (production) or the
	// shared dev token (dev/test) into an HttpOnly auth_token cookie;
	// POST /v1/web/logout clears that cookie. Both routes are PUBLIC
	// (no bearerAuthMiddleware) — they are entry points by definition.
	// Rate-limited to absorb credential-stuffing attempts; the per-IP
	// budget mirrors the OAuth start/callback budget for consistency.
	r.Group(func(r chi.Router) {
		r.Use(httprate.LimitByIP(20, 1*time.Minute))
		r.Post("/v1/web/login", deps.HandleWebLogin)
		r.Post("/v1/web/logout", deps.HandleWebLogout)
	})

	// Web UI routes (HTMX) - registered externally via RegisterWebRoutes
	if deps.WebHandler != nil {
		// Web UI group — auth required when AuthToken is configured.
		// When AuthToken is empty, webAuthMiddleware passes all requests (dev mode).
		r.Group(func(r chi.Router) {
			r.Use(deps.webAuthMiddleware)
			r.Get("/", deps.WebHandler.SearchPage)
			r.Post("/search", deps.WebHandler.SearchResults)
			r.Get("/artifact/{id}", deps.WebHandler.ArtifactDetail)
			r.Get("/evidence-bundles/new", deps.WebHandler.EvidenceBundleBuilderPage)
			r.Get("/digest", deps.WebHandler.DigestPage)
			r.Get("/topics", deps.WebHandler.TopicsPage)
			r.Get("/settings", deps.WebHandler.SettingsPage)
			r.Post("/settings/connectors/{id}/sync", deps.WebHandler.SyncConnectorHandler)
			r.Post("/settings/bookmarks/import", deps.WebHandler.BookmarkUploadHandler)
			r.Get("/status", deps.WebHandler.StatusPage)
			r.Get("/recommendations", deps.WebHandler.RecommendationsPage)
			r.Post("/recommendations/results", deps.WebHandler.RecommendationsResults)
			r.Get("/recommendations/preferences", deps.WebHandler.RecommendationPreferencesPage)
			r.Get("/recommendations/watches", deps.WebHandler.RecommendationWatchesPage)
			r.Get("/recommendations/watches/new", deps.WebHandler.RecommendationWatchEditorPage)
			r.Get("/recommendations/watches/{id}", deps.WebHandler.RecommendationWatchDetailPage)
			r.Get("/recommendations/watches/{id}/edit", deps.WebHandler.RecommendationWatchEditorPage)
			r.Post("/recommendations/watches/{id}/pause", deps.WebHandler.RecommendationWatchPauseAction)
			r.Post("/recommendations/watches/{id}/resume", deps.WebHandler.RecommendationWatchResumeAction)
			r.Post("/recommendations/watches/{id}/silence", deps.WebHandler.RecommendationWatchSilenceAction)
			r.Delete("/recommendations/watches/{id}", deps.WebHandler.RecommendationWatchDeleteAction)
			r.Post("/recommendations/{id}/feedback", deps.WebHandler.RecommendationFeedback)
			r.Get("/recommendations/{id}", deps.WebHandler.RecommendationDetail)
			r.Get("/recommendations/trip-dossier/{trip_id}", deps.WebHandler.TripDossierPage)
			r.Get("/notifications", deps.WebHandler.NotificationDashboard)
			r.Get("/notifications/sources", deps.WebHandler.NotificationSourcesPage)
			r.Get("/notifications/events", deps.WebHandler.NotificationEventsPage)
			r.Get("/notifications/incidents", deps.WebHandler.NotificationIncidentsPage)
			r.Get("/notifications/incidents/{incident_id}", deps.WebHandler.NotificationIncidentDetailPage)
			r.Get("/notifications/approvals", deps.WebHandler.NotificationApprovalsPage)
			r.Get("/notifications/approvals/{approval_id}", deps.WebHandler.NotificationApprovalDetailPage)
			r.Get("/notifications/suppressions", deps.WebHandler.NotificationSuppressionsPage)
			r.Get("/notifications/summary", deps.WebHandler.NotificationSummaryPage)
			r.Get("/notifications/outputs", deps.WebHandler.NotificationOutputsPage)

			// Knowledge layer web routes
			r.Get("/knowledge", deps.WebHandler.KnowledgeDashboard)
			r.Get("/knowledge/concepts", deps.WebHandler.ConceptsList)
			r.Get("/knowledge/concepts/{id}", deps.WebHandler.ConceptDetail)
			r.Get("/knowledge/entities", deps.WebHandler.EntitiesList)
			r.Get("/knowledge/entities/{id}", deps.WebHandler.EntityDetail)
			r.Get("/knowledge/lint", deps.WebHandler.LintReport)
			r.Get("/knowledge/lint/{id}", deps.WebHandler.LintFindingDetail)
		})
	}

	// PWA routes (spec 033) — no auth required, PWA must be publicly installable
	r.Route("/pwa", func(r chi.Router) {
		r.Post("/share", deps.PWAShareHandler)
		r.Handle("/*", http.StripPrefix("/pwa", pwaFileServer()))
	})

	// Spec 037 Scope 8 — Agent Operator UI (admin web routes).
	// Behind webAuthMiddleware so the same Bearer/cookie auth as the
	// rest of the admin web applies. Routes mirror the
	// `smackerel agent ...` CLI subcommands one-for-one.
	if deps.AgentAdminHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(deps.webAuthMiddleware)
			r.Route("/admin/agent", func(r chi.Router) {
				r.Get("/traces", deps.AgentAdminHandler.TracesIndex)
				r.Get("/traces/{id}", deps.AgentAdminHandler.TracesShow)
				r.Get("/scenarios", deps.AgentAdminHandler.ScenariosIndex)
				r.Get("/scenarios/{id}", deps.AgentAdminHandler.ScenariosShow)
				r.Get("/tools", deps.AgentAdminHandler.ToolsIndex)
				r.Get("/tools/{name}", deps.AgentAdminHandler.ToolsShow)
			})
		})
	}

	// Spec 044 Scope 03 — Per-user PASETO admin token-management UI.
	// Single static HTML page that calls the existing Scope 02
	// /v1/auth/* admin REST endpoints via fetch() with same-origin
	// credentials. Behind bearerAuthMiddleware so:
	//   - production callers must supply a per-user PASETO bearer
	//     (header) or auth_token cookie (set by /v1/web/login);
	//   - dev/test callers fall through the same shared-token /
	//     empty-token branches the rest of the admin REST surface
	//     uses.
	// The /v1/auth/* REST endpoints independently enforce the
	// callerIsAdmin gate, so non-admin authenticated callers can
	// load the page but XHR mutations will return 403.
	r.Group(func(r chi.Router) {
		r.Use(deps.bearerAuthMiddleware)
		r.Get("/admin/auth/tokens", deps.HandleAdminTokensUI)
	})

	if deps.AgentInvokeHandler != nil || deps.DriveHandlers != nil || deps.PhotosHandlers != nil || deps.DriveRulesHandlers != nil || deps.DriveSaveHandlers != nil || deps.DriveConfirmationsHandlers != nil || deps.AuthAdminHandlers != nil {
		r.Route("/v1", func(r chi.Router) {
			r.Use(middleware.Throttle(100))

			if deps.DriveHandlers != nil {
				// Spec 044 Scope 02 (MIT-038-S-003) — drive Connect must
				// derive owner_user_id from the authenticated session in
				// production. Wrap the drive routes in bearerAuthMiddleware
				// so the session is attached before the handler runs.
				// OAuthCallback stays unauthenticated because it is invoked
				// by the upstream OAuth provider redirect, which carries
				// no bearer token.
				r.Group(func(r chi.Router) {
					r.Use(deps.bearerAuthMiddleware)
					r.Get("/connectors/drive", deps.DriveHandlers.ListConnectors)
					r.Post("/connectors/drive/connect", deps.DriveHandlers.Connect)
					r.Get("/connectors/drive/connection/{id}", deps.DriveHandlers.GetConnection)
					r.Get("/connectors/drive/connection/{id}/skipped", deps.DriveHandlers.GetSkippedBlocked)
					r.Get("/drive/artifacts/{id}", deps.DriveHandlers.GetArtifactDetail)
				})
				r.Get("/connectors/drive/oauth/callback", deps.DriveHandlers.OAuthCallback)
			}

			// Spec 038 Scope 5 — Save Rules CRUD + audit + dry-run.
			if deps.DriveRulesHandlers != nil {
				r.Group(func(r chi.Router) {
					r.Use(deps.bearerAuthMiddleware)
					r.Get("/drive/rules", deps.DriveRulesHandlers.List)
					r.Post("/drive/rules", deps.DriveRulesHandlers.Create)
					r.Get("/drive/rules/audit", deps.DriveRulesHandlers.Audit)
					r.Get("/drive/rules/{id}", deps.DriveRulesHandlers.Get)
					r.Put("/drive/rules/{id}", deps.DriveRulesHandlers.Update)
					r.Delete("/drive/rules/{id}", deps.DriveRulesHandlers.Delete)
					r.Post("/drive/rules/{id}/test", deps.DriveRulesHandlers.Test)
				})
			}

			// Spec 038 Scope 5 — POST /v1/drive/save + recent requests.
			if deps.DriveSaveHandlers != nil {
				r.Group(func(r chi.Router) {
					r.Use(deps.bearerAuthMiddleware)
					r.Post("/drive/save", deps.DriveSaveHandlers.Save)
					r.Get("/drive/save/requests", deps.DriveSaveHandlers.ListRequests)
				})
			}

			// Spec 038 Scope 6 — Low-confidence confirmation resolution.
			// Both web (Screen 11) and Telegram numbered replies route
			// through the same handler so the exactly-once contract
			// holds across channels.
			if deps.DriveConfirmationsHandlers != nil {
				r.Group(func(r chi.Router) {
					r.Use(deps.bearerAuthMiddleware)
					r.Get("/drive/confirmations/{id}", deps.DriveConfirmationsHandlers.Get)
					r.Post("/drive/confirmations/{id}", deps.DriveConfirmationsHandlers.Resolve)
				})
			}

			if deps.PhotosHandlers != nil {
				r.Group(func(r chi.Router) {
					r.Use(deps.bearerAuthMiddleware)
					r.Get("/photos/search", deps.PhotosHandlers.Search)
					r.Get("/photos/connectors", deps.PhotosHandlers.ListConnectors)
					r.Post("/photos/connectors", deps.PhotosHandlers.Connect)
					r.Post("/photos/connectors/test", deps.PhotosHandlers.TestConnector)
					r.Get("/photos/connectors/{id}", deps.PhotosHandlers.GetConnector)
					// Spec 040 Scope 3 — lifecycle, duplicates,
					// removal, and action-token confirmation.
					r.Post("/photos/actions/plan", deps.PhotosHandlers.PlanAction)
					r.Post("/photos/actions/confirm", deps.PhotosHandlers.ConfirmAction)
					r.Get("/photos/health/lifecycle", deps.PhotosHandlers.HealthLifecycle)
					r.Get("/photos/health/duplicates", deps.PhotosHandlers.HealthDuplicates)
					r.Get("/photos/health/duplicates/{id}", deps.PhotosHandlers.HealthDuplicatesGet)
					r.Post("/photos/health/duplicates/{id}/best-pick", deps.PhotosHandlers.SetClusterBestPick)
					r.Post("/photos/health/duplicates/{id}/resolve", deps.PhotosHandlers.ResolveCluster)
					r.Get("/photos/health/removal", deps.PhotosHandlers.HealthRemoval)
					r.Get("/photos/health/quality", deps.PhotosHandlers.HealthQuality)
					// Spec 040 Scope 5 — multi-provider capability
					// governance + photo health aggregate. Registered
					// BEFORE the /photos/{id} catch-all so the literal
					// `health` segment wins routing.
					r.Post("/photos/connectors/capabilities/{capability}/exercise", deps.PhotosHandlers.ExerciseCapability)
					r.Get("/photos/health", deps.PhotosHandlers.HealthAggregate)
					// Spec 040 Scope 4 — upload (Telegram/mobile/web)
					// + sensitivity reveal token mint.
					r.Post("/photos/upload", deps.PhotosHandlers.Upload)
					r.Post("/photos/{id}/reveal", deps.PhotosHandlers.MintReveal)
					// Catch-all photo lookups MUST be registered last
					// so `/photos/health` and `/photos/upload` resolve
					// to their literal handlers instead of being
					// mistaken for a UUID lookup.
					r.Get("/photos/{id}/preview", deps.PhotosHandlers.Preview)
					r.Get("/photos/{id}", deps.PhotosHandlers.GetPhoto)
				})
			}

			// Spec 037 Scope 9 — POST /v1/agent/invoke (end-user failure
			// surfaces). Behind bearer auth (same policy as /api/*) so callers
			// must authenticate; replies always carry a structured outcome
			// envelope per spec §UX.
			r.Group(func(r chi.Router) {
				if deps.AgentInvokeHandler != nil {
					r.Use(deps.bearerAuthMiddleware)
					r.Post("/agent/invoke", deps.AgentInvokeHandler.AgentInvokeHandlerFunc)
				}
			})

			// Spec 044 Scope 02 — admin auth surface (POST/GET /v1/auth/*).
			// Behind bearerAuthMiddleware so callers must authenticate;
			// each handler additionally enforces admin scope via
			// callerIsAdmin against the auth.Session attached by the
			// middleware. Routes mirror the cmd_auth.go subcommand
			// surface one-for-one. Per OQ-6 the bootstrap session is
			// always admin and the shared-token session is admin in
			// dev/test only (or in production with the
			// production_shared_token_fallback_enabled opt-in flag).
			if deps.AuthAdminHandlers != nil {
				r.Group(func(r chi.Router) {
					r.Use(deps.bearerAuthMiddleware)
					r.Post("/auth/users", deps.AuthAdminHandlers.HandleEnroll)
					r.Get("/auth/users", deps.AuthAdminHandlers.HandleListUsers)
					r.Post("/auth/users/{user_id}/rotate", deps.AuthAdminHandlers.HandleRotate)
					r.Post("/auth/tokens/{token_id}/revoke", deps.AuthAdminHandlers.HandleRevoke)
				})
			}
		})
	}

	return r
}

// structuredLogger is a middleware that logs requests with slog.
// Health check and heartbeat endpoints are excluded to reduce log noise.
func structuredLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip logging for health check, heartbeat, readiness, and metrics endpoints
		// to reduce log noise from high-frequency monitoring probes (SCN-023-08, C-023-C004).
		switch r.URL.Path {
		case "/api/health", "/ping", "/readyz", "/metrics":
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		reqID := middleware.GetReqID(r.Context())
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", reqID,
		)
	})
}

// securityHeadersMiddleware sets security headers on all responses.
// Covers OWASP recommended headers: CSP, clickjacking, MIME sniffing, referrer leakage, cache control.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' https://unpkg.com/htmx.org@1.9.12/ 'sha256-C7I7zL0TtdR86YSsw1T7pxobSVoQGAOH9Ua4apor8TI='; img-src 'self' data:; connect-src 'self'")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// extractBearerToken extracts the token used for bearer authentication.
//
// Lookup order:
//  1. Authorization header ("Authorization: Bearer <token>"). When the
//     header is present but malformed (missing scheme, wrong scheme,
//     empty token), the function returns "" without falling back to
//     the cookie — a malformed header is a client bug that must be
//     surfaced as a 401, not silently masked by the cookie.
//  2. Spec 044 Scope 03 — auth_token cookie fallback. The PWA POSTs
//     to /v1/web/login with a per-user PASETO (or shared dev token);
//     the login handler sets an HttpOnly+SameSite=Lax cookie
//     (Secure in production). Subsequent same-origin requests carry
//     the cookie automatically; bearerAuthMiddleware uses the cookie
//     value as the bearer token when no Authorization header is
//     present, so the PWA does not have to attach Authorization
//     headers to every fetch().
func extractBearerToken(r *http.Request) string {
	token, _ := extractBearerTokenWithSource(r)
	return token
}

// extractBearerTokenWithSource is the spec 044 Scope 04 metric-aware
// variant of extractBearerToken: it returns the token AND the
// transport source ("header" or "pwa_cookie") so the middleware can
// label the validation outcome metric. Callers that don't need the
// source label use the unlabeled extractBearerToken.
//
// When neither a header nor a cookie is present, the source is "" and
// the token is "" — the caller writes a 401 (`missing_token`).
func extractBearerTokenWithSource(r *http.Request) (token, source string) {
	if header := r.Header.Get("Authorization"); header != "" {
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			return "", "header"
		}
		return parts[1], "header"
	}
	if cookie, err := r.Cookie("auth_token"); err == nil && cookie.Value != "" {
		return cookie.Value, "pwa_cookie"
	}
	return "", ""
}

// matchBearerToken returns true if the request carries a Bearer token that
// matches expected using constant-time comparison.
func matchBearerToken(r *http.Request, expected string) bool {
	token := extractBearerToken(r)
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

// webAuthMiddleware checks authentication for web UI routes.
// Accepts Bearer token in Authorization header or auth_token cookie.
// If no AuthToken is configured, all requests are allowed (dev mode).
func (d *Dependencies) webAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.AuthToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check Authorization header (Bearer token)
		if matchBearerToken(r, d.AuthToken) {
			next.ServeHTTP(w, r)
			return
		}

		// Check auth_token cookie (for browser sessions)
		if cookie, err := r.Cookie("auth_token"); err == nil {
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(d.AuthToken)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}

		slog.Warn("web auth failure", "path", r.URL.Path, "remote_addr", r.RemoteAddr)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

// bearerAuthMiddleware checks Bearer token authentication for API routes.
//
// Spec 044 Scope 02 hot-path validation contract. Five branches in order:
//
//  1. Production AND auth.enabled — verify per-user PASETO v4.public via
//     auth.VerifyAndParse; consult RevocationCache; on success attach an
//     auth.Session{Source: SessionSourcePerUserToken}. On failure return
//     401 with a generic UNAUTHORIZED body (NFR-AUTH-007 — no token
//     material in the response). FR-AUTH-004 / NFR-AUTH-001 / NFR-AUTH-002.
//  2. Production AND auth.enabled AND production_shared_token_fallback_enabled
//     opt-in — fall back to constant-time shared-token compare with a
//     deprecation slog.Warn so operators can drain legacy clients during
//     migration. Attaches SessionSourceSharedToken (UserID="").
//  3. Dev/test shared-token compare — preserves the SMACKEREL_AUTH_TOKEN
//     ergonomic per FR-AUTH-015. Attaches SessionSourceSharedToken
//     (UserID="").
//  4. Dev empty-token bypass — preserves the today-ever lever at the
//     prior router.go lines 444–451. Attaches SessionSourceSharedToken
//     so downstream session lookups still resolve the (Session, ok)
//     tuple instead of returning ok=false.
//  5. MIT-040-S-004 production empty-token defense-in-depth — when
//     d.AuthToken == "" AND Environment == "production" AND no PASETO
//     surface configured, reject 401 (the wiring layer already fails
//     fast on this case; this is the second layer).
//
// Constant-time discipline (NFR-AUTH-008): the shared-token comparison
// uses subtle.ConstantTimeCompare; the PASETO v4.public verifier inside
// go-paseto uses constant-time signature primitives. The 401 error
// response body never names which validation step failed (SCN-AUTH-010).
func (d *Dependencies) bearerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Branch 5 first — empty AuthToken AND no per-user surface is
		// the dev-bypass lever; production-mode is the defense-in-depth
		// 401 (the loader already failed earlier; we belt-and-brace).
		perUserActive := d.Environment == "production" && d.AuthConfig.Enabled

		if d.AuthToken == "" && !perUserActive {
			if d.Environment == "production" {
				slog.Warn("bearer auth blocked",
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
					"reason", "auth not configured in production")
				metrics.AuthFailure.WithLabelValues("auth_not_configured").Inc()
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "auth not configured in production")
				return
			}
			// Dev empty-token bypass — attach a synthetic session so
			// downstream handlers that consult auth.SessionFromContext
			// see a (Session, ok=true) tuple. Source is SharedToken so
			// the dev/test claim-binding fallbacks honor it.
			ctx := auth.WithSession(r.Context(), auth.Session{
				Source: auth.SessionSourceSharedToken,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		token, source := extractBearerTokenWithSource(r)
		if token == "" {
			reason := "missing_token"
			if r.Header.Get("Authorization") != "" {
				reason = "invalid_format"
			}
			slog.Warn("bearer auth failure",
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
				"reason", reason)
			metrics.AuthFailure.WithLabelValues(reason).Inc()
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authentication required")
			return
		}
		if source == "" {
			source = "header"
		}

		// Branch 1 — production per-user PASETO path.
		if perUserActive {
			start := time.Now()
			parsed, err := auth.VerifyAndParse(token, d.AuthVerifyOptions)
			metrics.AuthValidationLatency.Observe(time.Since(start).Seconds())
			if err == nil {
				// Revocation lookup is sync.Map.Load — lock-free,
				// allocation-free for the common case.
				if d.RevocationCache != nil && d.RevocationCache.IsRevoked(parsed.TokenID) {
					slog.Warn("bearer auth failure",
						"path", r.URL.Path,
						"remote_addr", r.RemoteAddr,
						"reason", "revoked")
					metrics.AuthValidationOutcome.WithLabelValues("rejected_revoked", source).Inc()
					metrics.AuthFailure.WithLabelValues("revoked").Inc()
					writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authentication required")
					return
				}
				sess := auth.Session{
					UserID:    parsed.UserID,
					TokenID:   parsed.TokenID,
					KeyID:     parsed.KeyID,
					IssuedAt:  parsed.IssuedAt,
					ExpiresAt: parsed.ExpiresAt,
					Source:    auth.SessionSourcePerUserToken,
				}
				metrics.AuthValidationOutcome.WithLabelValues("accepted", source).Inc()
				next.ServeHTTP(w, r.WithContext(auth.WithSession(r.Context(), sess)))
				return
			}

			// Branch 2 — production opt-in shared-token fallback.
			if d.AuthConfig.ProductionSharedTokenFallbackEnabled &&
				d.AuthToken != "" &&
				subtle.ConstantTimeCompare([]byte(token), []byte(d.AuthToken)) == 1 {
				slog.Warn("production shared-token fallback used (deprecation pathway)",
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr)
				metrics.AuthLegacyFallbackUsed.WithLabelValues("production").Inc()
				sess := auth.Session{Source: auth.SessionSourceSharedToken}
				next.ServeHTTP(w, r.WithContext(auth.WithSession(r.Context(), sess)))
				return
			}

			// Classify the verifier error into one of the closed-set
			// outcome buckets for the metric label. The 401 body is
			// generic per NFR-AUTH-007.
			outcome := classifyVerifyError(err)
			slog.Warn("bearer auth failure",
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
				"reason", "paseto verify failed")
			metrics.AuthValidationOutcome.WithLabelValues(outcome, source).Inc()
			metrics.AuthFailure.WithLabelValues("paseto_verify_failed").Inc()
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authentication required")
			return
		}

		// Branch 3 — dev/test shared-token compare (preserved per FR-AUTH-015).
		if d.AuthToken != "" &&
			subtle.ConstantTimeCompare([]byte(token), []byte(d.AuthToken)) == 1 {
			sess := auth.Session{Source: auth.SessionSourceSharedToken}
			next.ServeHTTP(w, r.WithContext(auth.WithSession(r.Context(), sess)))
			return
		}

		slog.Warn("bearer auth failure",
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"reason", "invalid token")
		metrics.AuthFailure.WithLabelValues("shared_token_mismatch").Inc()
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authentication required")
	})
}

// classifyVerifyError buckets a VerifyAndParse error into one of the
// closed-set outcome labels for the AuthValidationOutcome metric.
// Spec 044 Scope 04 OQ-9: outcome label MUST be one of {accepted,
// rejected_revoked, rejected_expired, rejected_malformed,
// rejected_unknown_key}. Anything we cannot positively classify lands
// in `rejected_malformed` so the dashboard never sees an empty bucket.
func classifyVerifyError(err error) string {
	if err == nil {
		return "accepted"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "expired"), strings.Contains(msg, "not yet valid"), strings.Contains(msg, "nbf"):
		return "rejected_expired"
	case strings.Contains(msg, "unknown key"), strings.Contains(msg, "no public key"), strings.Contains(msg, "kid"):
		return "rejected_unknown_key"
	default:
		return "rejected_malformed"
	}
}
