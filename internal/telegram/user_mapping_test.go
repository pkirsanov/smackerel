// Spec 044 Scope 03 — unit coverage for the Telegram per-user
// claim-binding helpers (ParseUserMapping + Bot.resolveActorUserID).
package telegram

import (
	"errors"
	"testing"
)

func TestParseUserMapping(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    map[int64]string
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"single pair", "12345:alice", map[int64]string{12345: "alice"}, false},
		{"two pairs", "12345:alice,67890:bob", map[int64]string{12345: "alice", 67890: "bob"}, false},
		{"whitespace tolerated", "  12345 : alice , 67890 : bob ", map[int64]string{12345: "alice", 67890: "bob"}, false},
		{"negative chat_id (Telegram supergroup)", "-1001234567890:carol", map[int64]string{-1001234567890: "carol"}, false},
		{"missing colon", "12345-alice", nil, true},
		{"missing user_id", "12345:", nil, true},
		{"missing chat_id", ":alice", nil, true},
		{"non-numeric chat_id", "abc:alice", nil, true},
		{"duplicate chat_id", "12345:alice,12345:bob", nil, true},
		{"empty pair", "12345:alice,,67890:bob", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseUserMapping(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil; got=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len got=%d want=%d (got=%v want=%v)", len(got), len(tc.want), got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("got[%d]=%q want=%q", k, got[k], v)
				}
			}
		})
	}
}

func TestResolveActorUserID_Production_RejectsUnmappedChat(t *testing.T) {
	b := &Bot{
		environment: "production",
		userMapping: map[int64]string{12345: "alice"},
	}
	user, err := b.resolveActorUserID(99999)
	if !errors.Is(err, ErrNoUserMappingForChat) {
		t.Fatalf("err=%v want %v", err, ErrNoUserMappingForChat)
	}
	if user != "" {
		t.Errorf("user=%q want empty (rejection MUST NOT return a synthetic actor)", user)
	}
}

func TestResolveActorUserID_Production_AcceptsMappedChat(t *testing.T) {
	b := &Bot{
		environment: "production",
		userMapping: map[int64]string{12345: "alice"},
	}
	user, err := b.resolveActorUserID(12345)
	if err != nil {
		t.Fatalf("err=%v want nil", err)
	}
	if user != "alice" {
		t.Errorf("user=%q want alice", user)
	}
}

func TestResolveActorUserID_Production_EmptyMappingRejectsAll(t *testing.T) {
	b := &Bot{environment: "production", userMapping: nil}
	_, err := b.resolveActorUserID(12345)
	if !errors.Is(err, ErrNoUserMappingForChat) {
		t.Fatalf("err=%v want %v — production with empty mapping MUST reject every chat", err, ErrNoUserMappingForChat)
	}
}

func TestResolveActorUserID_Dev_AllowsMappedAndUnmapped(t *testing.T) {
	for _, env := range []string{"development", "test", ""} {
		t.Run(env, func(t *testing.T) {
			b := &Bot{
				environment: env,
				userMapping: map[int64]string{12345: "alice"},
			}
			// Mapped chat → returns user_id, no error.
			user, err := b.resolveActorUserID(12345)
			if err != nil {
				t.Fatalf("mapped chat err=%v", err)
			}
			if user != "alice" {
				t.Errorf("mapped user=%q want alice", user)
			}
			// Unmapped chat → empty string, no error (dev tolerates).
			user, err = b.resolveActorUserID(99999)
			if err != nil {
				t.Fatalf("unmapped dev chat err=%v want nil", err)
			}
			if user != "" {
				t.Errorf("unmapped dev user=%q want empty", user)
			}
		})
	}
}

// TestResolveActorUserID_Production_CaseInsensitiveEnv proves the
// production check is case-insensitive (defensive — in case the SST
// emits "Production" or "PRODUCTION" via a misconfiguration).
func TestResolveActorUserID_Production_CaseInsensitiveEnv(t *testing.T) {
	for _, env := range []string{"production", "Production", "PRODUCTION"} {
		t.Run(env, func(t *testing.T) {
			b := &Bot{environment: env, userMapping: nil}
			_, err := b.resolveActorUserID(12345)
			if !errors.Is(err, ErrNoUserMappingForChat) {
				t.Errorf("env=%q err=%v want %v", env, err, ErrNoUserMappingForChat)
			}
		})
	}
}

// TestResolveActorUserID_NilBot defends the helper from a nil
// receiver — defense-in-depth in case a future code path dispatches
// without a constructed bot (e.g., partial test fixture).
func TestResolveActorUserID_NilBot(t *testing.T) {
	var b *Bot
	_, err := b.resolveActorUserID(12345)
	if !errors.Is(err, ErrNoUserMappingForChat) {
		t.Errorf("nil bot err=%v want %v", err, ErrNoUserMappingForChat)
	}
}
