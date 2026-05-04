package main

import (
	"strings"
	"testing"

	"github.com/weiboz0/bridge/platform/internal/config"
)

// Plan 050: validate the DEV_SKIP_AUTH × APP_ENV startup guard.

func TestValidateDevAuthEnv(t *testing.T) {
	cases := []struct {
		name        string
		env         map[string]string
		expectError bool
		errSubstr   string
	}{
		{
			name:        "DEV_SKIP_AUTH unset → no error regardless of APP_ENV",
			env:         map[string]string{"APP_ENV": "production"},
			expectError: false,
		},
		{
			name:        "DEV_SKIP_AUTH set + APP_ENV unset → no error (treated as non-prod)",
			env:         map[string]string{"DEV_SKIP_AUTH": "admin"},
			expectError: false,
		},
		{
			name:        "DEV_SKIP_AUTH set + APP_ENV=development → no error",
			env:         map[string]string{"DEV_SKIP_AUTH": "admin", "APP_ENV": "development"},
			expectError: false,
		},
		{
			name:        "DEV_SKIP_AUTH set + APP_ENV=staging → no error (only `production` blocks)",
			env:         map[string]string{"DEV_SKIP_AUTH": "admin", "APP_ENV": "staging"},
			expectError: false,
		},
		{
			name:        "DEV_SKIP_AUTH=admin + APP_ENV=production → ERROR",
			env:         map[string]string{"DEV_SKIP_AUTH": "admin", "APP_ENV": "production"},
			expectError: true,
			errSubstr:   "refusing to start",
		},
		{
			name:        "DEV_SKIP_AUTH=<uuid> + APP_ENV=production → ERROR",
			env:         map[string]string{"DEV_SKIP_AUTH": "00000000-0000-0000-0000-000000000001", "APP_ENV": "production"},
			expectError: true,
			errSubstr:   "DEV_SKIP_AUTH",
		},
		// Plan 068 phase 1 — BRIDGE_HOST_EXPOSURE guard for tunneled hosts.
		{
			name: "DEV_SKIP_AUTH set + BRIDGE_HOST_EXPOSURE unset → no error (default localhost)",
			env: map[string]string{
				"DEV_SKIP_AUTH": "admin",
			},
		},
		{
			name: "DEV_SKIP_AUTH set + BRIDGE_HOST_EXPOSURE=localhost → no error (explicit localhost)",
			env: map[string]string{
				"DEV_SKIP_AUTH":         "admin",
				"BRIDGE_HOST_EXPOSURE":  "localhost",
			},
		},
		{
			name: "DEV_SKIP_AUTH set + BRIDGE_HOST_EXPOSURE=exposed → ERROR",
			env: map[string]string{
				"DEV_SKIP_AUTH":         "admin",
				"BRIDGE_HOST_EXPOSURE":  "exposed",
			},
			expectError: true,
			errSubstr:   "BRIDGE_HOST_EXPOSURE=exposed",
		},
		{
			name: "DEV_SKIP_AUTH set + BRIDGE_HOST_EXPOSURE=exposed + ALLOW_DEV_AUTH_OVER_TUNNEL=true → no error (escape hatch)",
			env: map[string]string{
				"DEV_SKIP_AUTH":              "admin",
				"BRIDGE_HOST_EXPOSURE":       "exposed",
				"ALLOW_DEV_AUTH_OVER_TUNNEL": "true",
			},
		},
		{
			name: "DEV_SKIP_AUTH unset + BRIDGE_HOST_EXPOSURE=exposed → no error (no bypass to guard)",
			env: map[string]string{
				"BRIDGE_HOST_EXPOSURE": "exposed",
			},
		},
		{
			name: "DEV_SKIP_AUTH set + BRIDGE_HOST_EXPOSURE=exposed + ALLOW_DEV_AUTH_OVER_TUNNEL=anything-else → ERROR (only 'true' opens the hatch)",
			env: map[string]string{
				"DEV_SKIP_AUTH":              "admin",
				"BRIDGE_HOST_EXPOSURE":       "exposed",
				"ALLOW_DEV_AUTH_OVER_TUNNEL": "yes",
			},
			expectError: true,
			errSubstr:   "BRIDGE_HOST_EXPOSURE=exposed",
		},
		{
			name: "APP_ENV=production guard wins over BRIDGE_HOST_EXPOSURE escape hatch",
			env: map[string]string{
				"DEV_SKIP_AUTH":              "admin",
				"APP_ENV":                    "production",
				"BRIDGE_HOST_EXPOSURE":       "exposed",
				"ALLOW_DEV_AUTH_OVER_TUNNEL": "true",
			},
			expectError: true,
			errSubstr:   "APP_ENV=production",
		},
		// Plan 068 phase 1 — typo tolerance (Codex post-impl pass-1).
		{
			name: "BRIDGE_HOST_EXPOSURE=EXPOSED (uppercase) → still triggers ERROR (case-insensitive normalization)",
			env: map[string]string{
				"DEV_SKIP_AUTH":        "admin",
				"BRIDGE_HOST_EXPOSURE": "EXPOSED",
			},
			expectError: true,
			errSubstr:   "BRIDGE_HOST_EXPOSURE=exposed",
		},
		{
			name: "BRIDGE_HOST_EXPOSURE=' exposed ' (whitespace) → still triggers ERROR (trim + normalize)",
			env: map[string]string{
				"DEV_SKIP_AUTH":        "admin",
				"BRIDGE_HOST_EXPOSURE": "  exposed  ",
			},
			expectError: true,
			errSubstr:   "BRIDGE_HOST_EXPOSURE=exposed",
		},
		{
			name: "BRIDGE_HOST_EXPOSURE=Localhost (mixed case) → no error (normalizes to localhost)",
			env: map[string]string{
				"DEV_SKIP_AUTH":        "admin",
				"BRIDGE_HOST_EXPOSURE": "Localhost",
			},
		},
		{
			name: "BRIDGE_HOST_EXPOSURE=public (unknown value) → ERROR rather than silent pass-through",
			env: map[string]string{
				"DEV_SKIP_AUTH":        "admin",
				"BRIDGE_HOST_EXPOSURE": "public",
			},
			expectError: true,
			errSubstr:   "unrecognized",
		},
		{
			name: "BRIDGE_HOST_EXPOSURE=tunneled (typo for 'exposed') → ERROR (typo doesn't silently bypass)",
			env: map[string]string{
				"DEV_SKIP_AUTH":        "admin",
				"BRIDGE_HOST_EXPOSURE": "tunneled",
			},
			expectError: true,
			errSubstr:   "unrecognized",
		},
		{
			name: "ALLOW_DEV_AUTH_OVER_TUNNEL=TRUE (uppercase) → opens hatch (case-insensitive)",
			env: map[string]string{
				"DEV_SKIP_AUTH":              "admin",
				"BRIDGE_HOST_EXPOSURE":       "exposed",
				"ALLOW_DEV_AUTH_OVER_TUNNEL": "TRUE",
			},
		},
		{
			name: "ALLOW_DEV_AUTH_OVER_TUNNEL='1' → does NOT open hatch (only literal 'true' / 'TRUE' allowed)",
			env: map[string]string{
				"DEV_SKIP_AUTH":              "admin",
				"BRIDGE_HOST_EXPOSURE":       "exposed",
				"ALLOW_DEV_AUTH_OVER_TUNNEL": "1",
			},
			expectError: true,
			errSubstr:   "BRIDGE_HOST_EXPOSURE=exposed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			getEnv := func(k string) string { return tc.env[k] }
			err := validateDevAuthEnv(getEnv)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("expected error containing %q, got: %v", tc.errSubstr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Plan 065 Phase 1: validate the BRIDGE_SESSION_AUTH × secret-presence
// startup guard. Refusing to boot loud > silently 503'ing every
// authenticated request.
func TestValidateBridgeSessionEnv(t *testing.T) {
	cases := []struct {
		name        string
		cfg         *config.Config
		expectError bool
		errSubstr   string
	}{
		{
			name: "flag OFF + everything empty → no error (dormant)",
			cfg: &config.Config{
				BridgeSession: config.BridgeSessionConfig{
					AuthFlag:       false,
					Secrets:        nil,
					InternalBearer: "",
				},
			},
		},
		{
			name: "flag OFF + secrets set → no error",
			cfg: &config.Config{
				BridgeSession: config.BridgeSessionConfig{
					AuthFlag:       false,
					Secrets:        []string{"s1"},
					InternalBearer: "b",
				},
			},
		},
		{
			name: "flag ON + secrets set + bearer set → no error",
			cfg: &config.Config{
				BridgeSession: config.BridgeSessionConfig{
					AuthFlag:       true,
					Secrets:        []string{"signing-secret"},
					InternalBearer: "internal-bearer",
				},
			},
		},
		{
			name: "flag ON + secrets EMPTY → ERROR",
			cfg: &config.Config{
				BridgeSession: config.BridgeSessionConfig{
					AuthFlag:       true,
					Secrets:        nil,
					InternalBearer: "internal-bearer",
				},
			},
			expectError: true,
			errSubstr:   "BRIDGE_SESSION_SECRETS",
		},
		{
			name: "flag ON + bearer EMPTY → ERROR",
			cfg: &config.Config{
				BridgeSession: config.BridgeSessionConfig{
					AuthFlag:       true,
					Secrets:        []string{"signing-secret"},
					InternalBearer: "",
				},
			},
			expectError: true,
			errSubstr:   "BRIDGE_INTERNAL_SECRET",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBridgeSessionEnv(tc.cfg)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("expected error containing %q, got: %v", tc.errSubstr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
