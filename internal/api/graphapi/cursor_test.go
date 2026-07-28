package graphapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// TestEncodeDecodeCursor_Roundtrip — SCN-080-11 regression: a cursor
// produced by Encode must decode back to the same payload.
func TestEncodeDecodeCursor_Roundtrip(t *testing.T) {
	codec := mustCodec(t, "test-secret-roundtrip-32-bytes!!")
	in := CursorPayload{
		Resource:    "topics",
		LastSortKey: "2026-06-03T12:34:56Z",
		LastID:      "topic-42",
		Offset:      100,
		Checksum:    "abc123",
	}
	enc, err := codec.Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.HasPrefix(enc, cursorVersion+".") {
		t.Errorf("encoded cursor missing %q prefix: %q", cursorVersion+".", enc)
	}
	out, err := codec.Decode(enc)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out != in {
		t.Errorf("round-trip mismatch:\n  want %+v\n  got  %+v", in, out)
	}
}

// TestDecodeCursor_RejectsGarbage — SCN-080-11: malformed cursors of
// every shape must return ErrMalformedCursor. Each case is the
// adversarial form a real client (or attacker) might produce.
func TestDecodeCursor_RejectsGarbage(t *testing.T) {
	codec := mustCodec(t, "test-secret-garbage-cases-32!!")
	cases := map[string]string{
		"empty":               "",
		"plain garbage":       "not-a-real-cursor",
		"wrong segment count": "v1.onlytwosegments",
		"too many segments":   "v1.a.b.c.d",
		"unknown version":     "v9.aaaa.bbbb",
		"non-base64 payload":  "v1.@@@@.bbbb",
		"non-base64 mac":      "v1.aaaa.@@@@",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := codec.Decode(in)
			if !errors.Is(err, ErrMalformedCursor) {
				t.Errorf("Decode(%q) err = %v; want ErrMalformedCursor", in, err)
			}
		})
	}
}

// TestDecodeCursor_RejectsTamper — adversarial: flip one byte of the
// HMAC and verify Decode rejects via hmac.Equal constant-time compare.
func TestDecodeCursor_RejectsTamper(t *testing.T) {
	codec := mustCodec(t, "tamper-secret-bytes-32-chars-!!aa")
	enc, err := codec.Encode(CursorPayload{Resource: "people", LastID: "p1"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	parts := strings.Split(enc, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(parts))
	}
	// Flip the FIRST byte of the base64url MAC segment. Flipping a
	// trailing byte can land entirely in base64 padding bits and
	// silently round-trip to the same decoded bytes; flipping a
	// leading byte guarantees the decoded MAC differs.
	mac := []byte(parts[2])
	mac[0] ^= 0x01
	// Ensure the flipped byte stays in the base64url alphabet so the
	// failure is "HMAC mismatch", not "base64 decode error".
	if mac[0] < 'A' || (mac[0] > 'Z' && mac[0] < 'a') || mac[0] > 'z' {
		mac[0] = 'A'
	}
	parts[2] = string(mac)
	tampered := strings.Join(parts, ".")
	if tampered == enc {
		t.Fatal("tamper produced identical cursor; test is degenerate")
	}
	if _, err := codec.Decode(tampered); !errors.Is(err, ErrMalformedCursor) {
		t.Errorf("Decode(tampered) err = %v; want ErrMalformedCursor", err)
	}
}

// TestDecodeCursor_RejectsCrossKeyForgery — a cursor signed with key A
// must not verify under key B. Guards against the "swap signing key,
// forget to invalidate old cursors" misconfiguration mode.
func TestDecodeCursor_RejectsCrossKeyForgery(t *testing.T) {
	codecA := mustCodec(t, "secret-A-secret-A-32-bytes!!aaaa")
	codecB := mustCodec(t, "secret-B-secret-B-32-bytes!!bbbb")
	enc, err := codecA.Encode(CursorPayload{Resource: "places", LastID: "pl1"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := codecB.Decode(enc); !errors.Is(err, ErrMalformedCursor) {
		t.Errorf("Decode under wrong key err = %v; want ErrMalformedCursor", err)
	}
}

// TestNewCursorCodec_RejectsEmptySecret — fail-loud guard: a codec
// constructed with an empty secret is unusable; the SST loader must
// catch this upstream, but the constructor double-checks.
func TestNewCursorCodec_RejectsEmptySecret(t *testing.T) {
	if _, err := NewCursorCodec(nil); err == nil {
		t.Error("NewCursorCodec(nil) returned no error; want fail-loud")
	}
	if _, err := NewCursorCodec([]byte{}); err == nil {
		t.Error("NewCursorCodec([]byte{}) returned no error; want fail-loud")
	}
}

func mustCodec(t *testing.T, secret string) *CursorCodec {
	t.Helper()
	c, err := NewCursorCodec([]byte(secret))
	if err != nil {
		t.Fatalf("NewCursorCodec: %v", err)
	}
	return c
}

// --- T080-06-CURSOR / SCN-080-001-06 ---------------------------------
//
// Authoritative home for the "a non-terminal page must never be
// rendered as terminal" contract. Every paginated graphapi family is
// exercised from the single table below, so the contract has exactly
// one place to change and one place to break.

// testCursorSecret is the HMAC signing key the graphapi handler unit
// tests sign with (newTopicsTestHandlers / newEdgesTestHandlers). Named
// here so the value-safety arm can assert it never reaches a response
// body.
const testCursorSecret = "test-secret-for-graphapi-handlers"

// cursorGuardPageLimit is the ?limit= every probe below requests.
// Holding the limit fixed makes terminality a property of the FIXTURE
// SIZE — exactly as it is in production — rather than a flag the test
// sets on the handler.
const cursorGuardPageLimit = 2

// cursorGuardFixtureRows sizes a fixture so the served page has the
// requested terminality.
func cursorGuardFixtureRows(nonTerminal bool) int {
	if nonTerminal {
		return cursorGuardPageLimit + 1 // a row remains behind the page ⇒ hasNext=true
	}
	return cursorGuardPageLimit // the page consumes the fixture ⇒ hasNext=false
}

// unusableCursorCodecs enumerates the two ways a codec can fail to
// produce a cursor at runtime: it was never wired (nil), and it was
// constructed outside NewCursorCodec so it carries no signing secret.
// (*CursorCodec).Encode has a nil-receiver guard, so both return an
// error instead of panicking.
func unusableCursorCodecs() map[string]*CursorCodec {
	return map[string]*CursorCodec{
		"nil codec":            nil,
		"codec without secret": {},
	}
}

// cursorGuardProbe describes one probe of a family's list endpoint.
type cursorGuardProbe struct {
	// nonTerminal makes the source report hasNext=true, so the handler
	// owes the client a continuation cursor.
	nonTerminal bool
	// codec replaces the working codec the test handler ships with, but
	// only when replaceCodec is set. A nil codec is itself one of the
	// failure modes under test, so nil cannot double as "leave the
	// working codec alone".
	codec        *CursorCodec
	replaceCodec bool
}

// cursorGuardFamily adapts one paginated graphapi family to the shared
// contract table: build a handler over a page of the requested shape,
// optionally swap its codec, issue that family's own list request, and
// read back the two success-envelope fields the contract constrains.
type cursorGuardFamily struct {
	name  string
	serve func(t *testing.T, probe cursorGuardProbe) *httptest.ResponseRecorder
	// page decodes a 200 success envelope into (itemCount, nextCursor).
	page func(t *testing.T, body []byte) (int, string)
}

// paginatedTopicsSource models the production pgx source: it serves at
// most `limit` rows from `offset` out of a fixture and reports
// hasNext=true whenever the fixture still holds rows beyond the page
// just served.
func paginatedTopicsSource(fixture []TopicRow) *stubTopicsSource {
	return &stubTopicsSource{
		listFn: func(_ context.Context, limit, offset int) ([]TopicRow, bool, error) {
			if offset >= len(fixture) {
				return []TopicRow{}, false, nil
			}
			end := offset + limit
			if end > len(fixture) {
				end = len(fixture)
			}
			return fixture[offset:end], end < len(fixture), nil
		},
	}
}

// paginatedEdgesSource is the edges-family counterpart of
// paginatedTopicsSource, with the same offset/limit/hasNext semantics.
func paginatedEdgesSource(fixture []EdgeRow) *stubEdgesSource {
	return &stubEdgesSource{
		listFn: func(_ context.Context, _, _ string, limit, offset int) ([]EdgeRow, bool, error) {
			if offset >= len(fixture) {
				return []EdgeRow{}, false, nil
			}
			end := offset + limit
			if end > len(fixture) {
				end = len(fixture)
			}
			return fixture[offset:end], end < len(fixture), nil
		},
	}
}

// cursorGuardFamilies is every paginated family the contract binds.
func cursorGuardFamilies() []cursorGuardFamily {
	return []cursorGuardFamily{
		{
			name: "topics",
			serve: func(t *testing.T, probe cursorGuardProbe) *httptest.ResponseRecorder {
				t.Helper()
				n := cursorGuardFixtureRows(probe.nonTerminal)
				fixture := make([]TopicRow, 0, n)
				for i := 1; i <= n; i++ {
					id := "T" + strconv.Itoa(i)
					fixture = append(fixture, TopicRow{ID: id, Label: "label-" + id})
				}
				h := newTopicsTestHandlers(t, paginatedTopicsSource(fixture))
				if probe.replaceCodec {
					h.Codec = probe.codec
				}
				rec := httptest.NewRecorder()
				mountTopicsRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
					"/api/topics?limit="+strconv.Itoa(cursorGuardPageLimit), nil))
				return rec
			},
			page: func(t *testing.T, body []byte) (int, string) {
				t.Helper()
				var resp topicsListResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("decode topics list envelope: %v", err)
				}
				return len(resp.Items), resp.NextCursor
			},
		},
		{
			name: "edges",
			serve: func(t *testing.T, probe cursorGuardProbe) *httptest.ResponseRecorder {
				t.Helper()
				n := cursorGuardFixtureRows(probe.nonTerminal)
				fixture := make([]EdgeRow, 0, n)
				for i := 1; i <= n; i++ {
					id := strconv.Itoa(i)
					fixture = append(fixture, EdgeRow{
						EdgeID: "e" + id, TargetKind: "topic",
						TargetID: "T" + id, TargetLabel: "travel-" + id,
					})
				}
				h := newEdgesTestHandlers(t, paginatedEdgesSource(fixture))
				if probe.replaceCodec {
					h.Codec = probe.codec
				}
				rec := httptest.NewRecorder()
				mountEdgesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
					"/api/graph/edges?source=artifact:A42&limit="+strconv.Itoa(cursorGuardPageLimit), nil))
				return rec
			},
			page: func(t *testing.T, body []byte) (int, string) {
				t.Helper()
				var resp edgesListResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("decode edges list envelope: %v", err)
				}
				return len(resp.Items), resp.NextCursor
			},
		},
	}
}

// TestNonTerminalPageCannotLoseCursorEncodeFailure is the authoritative
// T080-06-CURSOR / SCN-080-001-06 contract test.
//
// design.md §"Completeness Envelope" forbids rendering a non-terminal
// page as terminal, and §"Closed Read Outcomes" maps an internal
// projection inconsistency to `schema-error` → 500 `schema_error`. A
// page with rows still behind it whose continuation cursor cannot be
// produced is exactly that case.
//
// The defect this locks in: the list handlers previously used the
// fail-soft form `if encErr == nil { next = encoded }`, so a failed (or
// never wired) codec answered 200 with `nextCursor: ""` — which every
// client reads as "last page". That is silent data truncation.
// Restoring that form turns the adversarial arm RED.
//
// Three arms, run against every paginated family:
//
//	adversarial   non-terminal page + unusable codec ⇒ 500 schema_error;
//	              never 200, never a nextCursor field
//	scoped guard  terminal page + unusable codec ⇒ still 200, proving the
//	              guard is bound to hasNext and is not a blanket failure
//	value safety  the 500 body discloses no signing secret and no cursor
//	              material, with a working-codec control arm proving the
//	              codec alone caused the 500
func TestNonTerminalPageCannotLoseCursorEncodeFailure(t *testing.T) {
	for _, family := range cursorGuardFamilies() {
		t.Run(family.name, func(t *testing.T) {
			t.Run("adversarial_non_terminal_page_with_unusable_codec_is_500_schema_error", func(t *testing.T) {
				for codecName, codec := range unusableCursorCodecs() {
					t.Run(codecName, func(t *testing.T) {
						rec := family.serve(t, cursorGuardProbe{
							nonTerminal: true, codec: codec, replaceCodec: true,
						})

						if rec.Code == http.StatusOK {
							t.Fatalf("fail-soft regression: non-terminal page answered 200; a lost cursor must never look like the last page (body=%s)", rec.Body.String())
						}
						if rec.Code != http.StatusInternalServerError {
							t.Fatalf("want 500, got %d (body=%s)", rec.Code, rec.Body.String())
						}
						var env ErrorEnvelope
						if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
							t.Fatalf("decode error envelope: %v", err)
						}
						if env.Error.Code != CodeSchemaError {
							t.Fatalf("want code=%s, got %s", CodeSchemaError, env.Error.Code)
						}
						if env.Error.Message != ErrSchemaError.Message {
							t.Fatalf("want the generic ErrSchemaError message, got %q", env.Error.Message)
						}
						// The old fail-soft path emitted the SUCCESS
						// envelope with an empty cursor. The typed failure
						// must carry neither a nextCursor field (not even
						// an empty one) nor a partial items array.
						var raw map[string]any
						if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
							t.Fatalf("decode raw body: %v", err)
						}
						if _, present := raw["nextCursor"]; present {
							t.Fatalf("500 body must not carry a nextCursor field, got %s", rec.Body.String())
						}
						if _, present := raw["items"]; present {
							t.Fatalf("500 body must not carry a partial items array, got %s", rec.Body.String())
						}
					})
				}
			})

			t.Run("scoped_guard_terminal_page_with_unusable_codec_still_succeeds", func(t *testing.T) {
				for codecName, codec := range unusableCursorCodecs() {
					t.Run(codecName, func(t *testing.T) {
						rec := family.serve(t, cursorGuardProbe{
							nonTerminal: false, codec: codec, replaceCodec: true,
						})

						if rec.Code != http.StatusOK {
							t.Fatalf("terminal page must still be 200, got %d (body=%s)", rec.Code, rec.Body.String())
						}
						items, next := family.page(t, rec.Body.Bytes())
						if next != "" {
							t.Fatalf("terminal page must carry an empty nextCursor, got %q", next)
						}
						if items != cursorGuardPageLimit {
							t.Fatalf("want %d items on the terminal page, got %d", cursorGuardPageLimit, items)
						}
					})
				}
			})

			t.Run("value_safety_500_body_discloses_no_secret_or_cursor_material", func(t *testing.T) {
				// Control arm: same fixture, same request, ONLY the codec
				// differs. A working codec must still yield 200 with a
				// usable cursor — proof the guard did not break pagination,
				// and proof the 500 below is caused by the codec alone.
				okRec := family.serve(t, cursorGuardProbe{nonTerminal: true})
				if okRec.Code != http.StatusOK {
					t.Fatalf("working codec: want 200, got %d (body=%s)", okRec.Code, okRec.Body.String())
				}
				_, realCursor := family.page(t, okRec.Body.Bytes())
				if realCursor == "" {
					t.Fatal("working codec: non-terminal page must carry a non-empty nextCursor")
				}

				errRec := family.serve(t, cursorGuardProbe{
					nonTerminal: true, codec: &CursorCodec{}, replaceCodec: true,
				})
				if errRec.Code != http.StatusInternalServerError {
					t.Fatalf("unusable codec: want 500, got %d (body=%s)", errRec.Code, errRec.Body.String())
				}
				body := errRec.Body.String()
				for _, leaked := range []string{
					testCursorSecret,    // the HMAC signing key
					realCursor,          // a real cursor for this very page
					cursorVersion + ".", // any encoded cursor prefix
					"hmac",
					"secret",
				} {
					if strings.Contains(body, leaked) {
						t.Fatalf("500 body leaked cursor/secret material %q: %s", leaked, body)
					}
				}
			})
		})
	}
}
