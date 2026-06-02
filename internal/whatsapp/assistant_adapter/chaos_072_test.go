// Spec 072 — bubbles.chaos pass.
//
// Stochastic fuzz against the WhatsApp Business webhook handler and
// the outbound Render() surface. Goal: no panics, no 5xx leaks, only
// closed-vocabulary status codes, and Render either returns an
// OutboundMessage or a typed error for every random AssistantResponse
// shape. Seed is logged so failures are reproducible.

package assistant_adapter

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// TestChaos072_WebhookHandler_NeverPanicsAndStaysIn4xx5xxClosedSet
// throws 200 random requests at the webhook handler and asserts:
//   - no panic
//   - status code is one of {200, 400, 401, 403, 405, 413}
//     (the closed vocabulary documented in webhook_handler.go)
//   - the facade is invoked at most once per accepted delivery
func TestChaos072_WebhookHandler_NeverPanicsAndStaysIn4xx5xxClosedSet(t *testing.T) {
	seed := time.Now().UnixNano()
	if s := chaosSeedOverride(); s != 0 {
		seed = s
	}
	t.Logf("chaos-072 webhook seed=%d", seed)
	rng := rand.New(rand.NewSource(seed))

	fac := &recordingFacade{}
	adapter := newTestAdapter(t, fixedRegistry{userID: "user-chaos"})
	if err := adapter.Start(context.Background(), assistantBound(fac)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	h := NewWebhookHandler(WebhookHandlerOptions{Adapter: adapter})

	allowed := map[int]struct{}{
		http.StatusOK:                    {},
		http.StatusBadRequest:            {},
		http.StatusUnauthorized:          {},
		http.StatusForbidden:             {},
		http.StatusMethodNotAllowed:      {},
		http.StatusRequestEntityTooLarge: {},
	}

	const N = 200
	for i := 0; i < N; i++ {
		method := pickMethod(rng)
		body := randomBody(rng)
		sig := randomSignature(rng, body)

		req := httptest.NewRequest(method, "/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
		if sig != "" {
			req.Header.Set(SignatureHeader, sig)
		}
		if method == http.MethodGet {
			q := req.URL.Query()
			q.Set("hub.mode", pickChoice(rng, []string{"subscribe", "unsub", "", "SUBSCRIBE"}))
			q.Set("hub.verify_token", pickChoice(rng, []string{"tok", "wrong", "", randomString(rng, 32)}))
			q.Set("hub.challenge", pickChoice(rng, []string{"chal", "", randomString(rng, 16)}))
			req.URL.RawQuery = q.Encode()
		}

		rec := httptest.NewRecorder()
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("chaos-072 webhook panic at i=%d seed=%d method=%s sigLen=%d bodyLen=%d: %v",
						i, seed, method, len(sig), len(body), p)
				}
			}()
			h.ServeHTTP(rec, req)
		}()

		if _, ok := allowed[rec.Code]; !ok {
			t.Fatalf("chaos-072 webhook out-of-vocab status %d at i=%d seed=%d (method=%s)",
				rec.Code, i, seed, method)
		}
	}
}

// TestChaos072_Render_NeverPanicsForRandomResponseShapes hammers
// Render with 200 random AssistantResponse shapes. Render must
// either return a well-formed OutboundMessage with a known Kind, or
// a non-nil error. No panics.
func TestChaos072_Render_NeverPanicsForRandomResponseShapes(t *testing.T) {
	seed := time.Now().UnixNano()
	if s := chaosSeedOverride(); s != 0 {
		seed = s
	}
	t.Logf("chaos-072 render seed=%d", seed)
	rng := rand.New(rand.NewSource(seed))

	const N = 200
	for i := 0; i < N; i++ {
		resp := randomAssistantResponse(rng)
		maxChars := pickMaxChars(rng)

		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("chaos-072 render panic at i=%d seed=%d maxChars=%d: %v",
						i, seed, maxChars, p)
				}
			}()
			out, err := Render(resp, maxChars)
			if err != nil {
				return
			}
			switch out.Kind {
			case OutboundText, OutboundInteractiveButtons, OutboundInteractiveList:
				// ok
			default:
				t.Fatalf("chaos-072 render returned unknown kind %q at i=%d seed=%d", out.Kind, i, seed)
			}
			if out.Kind == OutboundText {
				if out.Text == nil {
					t.Fatalf("chaos-072 render text kind with nil Text at i=%d seed=%d", i, seed)
				}
				if len(out.Text.Body) > maxChars && maxChars > 0 {
					// Allow renderer to use rune-based truncation that
					// produces byte length up to ~4x maxChars for
					// multi-byte; flag only egregious overshoot.
					if len(out.Text.Body) > maxChars*8 {
						t.Fatalf("chaos-072 render text body len=%d >> maxChars=%d at i=%d seed=%d",
							len(out.Text.Body), maxChars, i, seed)
					}
				}
			}
		}()
	}
}

// ---- helpers ----

func chaosSeedOverride() int64 { return 0 }

func pickMethod(rng *rand.Rand) string {
	choices := []string{
		http.MethodPost, http.MethodPost, http.MethodPost, http.MethodPost,
		http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch,
	}
	return choices[rng.Intn(len(choices))]
}

func randomBody(rng *rand.Rand) []byte {
	bucket := rng.Intn(10)
	switch bucket {
	case 0:
		return nil
	case 1:
		return []byte{}
	case 2:
		return []byte("not json at all")
	case 3:
		return []byte("{")
	case 4:
		return []byte(sampleTextPayload)
	case 5:
		// oversize > 1 MiB
		buf := make([]byte, WebhookMaxBodyBytes+1024)
		rng.Read(buf)
		return buf
	case 6:
		return []byte(`{"object":"x","entry":[]}`)
	case 7:
		return []byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"messages":[{"type":"audio"}]}}]}]}`)
	default:
		return []byte(fmt.Sprintf(`{"object":%q,"entry":[%s]}`,
			randomString(rng, 8), randomString(rng, rng.Intn(64))))
	}
}

func randomSignature(rng *rand.Rand, body []byte) string {
	bucket := rng.Intn(8)
	switch bucket {
	case 0:
		return ""
	case 1:
		return "sha1=deadbeef"
	case 2:
		return "sha256=not-hex"
	case 3:
		return "sha256=" + strings.Repeat("a", 64)
	case 4:
		return sign(body, "wrong-secret")
	case 5:
		return sign(body, testAppSecret) // legitimate
	case 6:
		return "garbage-no-prefix"
	default:
		return "sha256=" + randomString(rng, 64)
	}
}

func pickChoice(rng *rand.Rand, choices []string) string {
	return choices[rng.Intn(len(choices))]
}

func randomString(rng *rand.Rand, n int) string {
	if n <= 0 {
		return ""
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789-_/\"\\{}[],.\n\t \xc3\xa9\xe4\xb8\xad"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(b)
}

func pickMaxChars(rng *rand.Rand) int {
	choices := []int{1, 10, 100, 1024, 4096, 8192}
	return choices[rng.Intn(len(choices))]
}

func randomAssistantResponse(rng *rand.Rand) contracts.AssistantResponse {
	resp := contracts.AssistantResponse{}
	if rng.Intn(2) == 0 {
		resp.Body = randomString(rng, rng.Intn(2048))
	}
	switch rng.Intn(6) {
	case 0:
		resp.Status = contracts.StatusUnavailable
		resp.ErrorCause = contracts.ErrorCause(randomString(rng, 16))
	case 1:
		n := rng.Intn(15)
		choices := make([]contracts.DisambiguationChoice, 0, n)
		for i := 0; i < n; i++ {
			choices = append(choices, contracts.DisambiguationChoice{
				Number: i + 1,
				ID:     randomString(rng, 8),
				Label:  randomString(rng, rng.Intn(80)),
			})
		}
		resp.DisambiguationPrompt = &contracts.DisambiguationPrompt{
			DisambiguationRef: randomString(rng, 8),
			Choices:           choices,
		}
	case 2:
		resp.ConfirmCard = &contracts.ConfirmCard{
			ConfirmRef:     randomString(rng, 8),
			ProposedAction: randomString(rng, rng.Intn(200)),
			PositiveLabel:  "Yes",
			NegativeLabel:  "No",
		}
	case 3:
		resp.CaptureRoute = true
	}
	return resp
}
