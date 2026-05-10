package auth

import (
	"strings"
	"testing"
)

// Spec 044 Scope 01 — T1-09 unit test for the runtime startup auth
// validation guard. Mirrors the wiring-layer defense-in-depth check
// at cmd/core/wiring.go (configureLogging) and proves every failure
// branch is covered by a focused test, plus that disabled / non-
// production combinations short-circuit to nil.
//
// Adversarial cases (each MUST fail loudly):
//   - production + enabled + empty signing key
//   - production + enabled + empty key id
//   - production + enabled + empty hashing key
//   - production + enabled + hashing key == signing key
//
// Permitted cases (each MUST return nil):
//   - production + disabled (empty material is fine)
//   - development + enabled (bootstrap-time flow)
//   - test + enabled (bootstrap-time flow)
//   - production + enabled + all material distinct + non-empty
func TestValidateRuntimeAuthStartup(t *testing.T) {
	good := RuntimeAuthConfig{
		Enabled:                 true,
		SigningActivePrivateKey: "0011223344556677889900112233445566778899001122334455667788990011",
		SigningActiveKeyID:      "key-2026-05",
		AtRestHashingKey:        "ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100",
	}

	cases := []struct {
		name        string
		environment string
		cfg         RuntimeAuthConfig
		wantErr     bool
		wantSubstr  string
	}{
		{
			name:        "production+enabled+well-formed permitted",
			environment: "production",
			cfg:         good,
			wantErr:     false,
		},
		{
			name:        "production+disabled bypasses validation",
			environment: "production",
			cfg: RuntimeAuthConfig{
				Enabled:                 false,
				SigningActivePrivateKey: "",
				SigningActiveKeyID:      "",
				AtRestHashingKey:        "",
			},
			wantErr: false,
		},
		{
			name:        "development+enabled+empty material permitted (bootstrap-time)",
			environment: "development",
			cfg: RuntimeAuthConfig{
				Enabled:                 true,
				SigningActivePrivateKey: "",
				SigningActiveKeyID:      "",
				AtRestHashingKey:        "",
			},
			wantErr: false,
		},
		{
			name:        "test+enabled+empty material permitted (bootstrap-time)",
			environment: "test",
			cfg: RuntimeAuthConfig{
				Enabled:                 true,
				SigningActivePrivateKey: "",
				SigningActiveKeyID:      "",
				AtRestHashingKey:        "",
			},
			wantErr: false,
		},
		{
			name:        "production+enabled+empty signing key fails loudly",
			environment: "production",
			cfg: RuntimeAuthConfig{
				Enabled:                 true,
				SigningActivePrivateKey: "",
				SigningActiveKeyID:      good.SigningActiveKeyID,
				AtRestHashingKey:        good.AtRestHashingKey,
			},
			wantErr:    true,
			wantSubstr: "AUTH_SIGNING_ACTIVE_PRIVATE_KEY",
		},
		{
			name:        "production+enabled+empty key id fails loudly",
			environment: "production",
			cfg: RuntimeAuthConfig{
				Enabled:                 true,
				SigningActivePrivateKey: good.SigningActivePrivateKey,
				SigningActiveKeyID:      "",
				AtRestHashingKey:        good.AtRestHashingKey,
			},
			wantErr:    true,
			wantSubstr: "AUTH_SIGNING_ACTIVE_KEY_ID",
		},
		{
			name:        "production+enabled+empty hashing key fails loudly",
			environment: "production",
			cfg: RuntimeAuthConfig{
				Enabled:                 true,
				SigningActivePrivateKey: good.SigningActivePrivateKey,
				SigningActiveKeyID:      good.SigningActiveKeyID,
				AtRestHashingKey:        "",
			},
			wantErr:    true,
			wantSubstr: "AUTH_AT_REST_HASHING_KEY",
		},
		{
			name:        "production+enabled+hashing key equals signing key fails loudly (OQ-8)",
			environment: "production",
			cfg: RuntimeAuthConfig{
				Enabled:                 true,
				SigningActivePrivateKey: good.SigningActivePrivateKey,
				SigningActiveKeyID:      good.SigningActiveKeyID,
				AtRestHashingKey:        good.SigningActivePrivateKey,
			},
			wantErr:    true,
			wantSubstr: "must differ",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRuntimeAuthStartup(tc.environment, tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateRuntimeAuthStartup(%q, %+v) = nil, want error containing %q", tc.environment, tc.cfg, tc.wantSubstr)
				}
				if !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Errorf("error message %q does not contain expected substring %q", err.Error(), tc.wantSubstr)
				}
			} else if err != nil {
				t.Errorf("ValidateRuntimeAuthStartup(%q, %+v) returned unexpected error: %v", tc.environment, tc.cfg, err)
			}
		})
	}
}
