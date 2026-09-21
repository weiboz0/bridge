package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
				"DEV_SKIP_AUTH":        "admin",
				"BRIDGE_HOST_EXPOSURE": "localhost",
			},
		},
		{
			name: "DEV_SKIP_AUTH set + BRIDGE_HOST_EXPOSURE=exposed → ERROR",
			env: map[string]string{
				"DEV_SKIP_AUTH":        "admin",
				"BRIDGE_HOST_EXPOSURE": "exposed",
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

// Plan 094 Phase 14: the E2E stack attestation is a test-only surface that
// answers an anonymous caller with one database query per well-formed nonce.
// Two boot guards keep it off production and off internet-facing hosts that
// have not opted in a second time. Both are pure, so they are table-tested
// here beside the DEV_SKIP_AUTH guard.

type e2eStackEnvCase struct {
	name            string
	enabled         bool
	production      bool
	exposure        string
	allowOverTunnel bool
	expectError     bool
	errSubstrs      []string
	forbidSubstrs   []string
}

func runE2EStackEnvCases(t *testing.T, cases []e2eStackEnvCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateE2EStackEnv(tc.enabled, tc.production, tc.exposure, tc.allowOverTunnel)
			if !tc.expectError {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected a refusal to start, got nil")
			}
			for _, substr := range tc.errSubstrs {
				if !strings.Contains(err.Error(), substr) {
					t.Errorf("expected error containing %q, got: %v", substr, err)
				}
			}
			for _, substr := range tc.forbidSubstrs {
				if strings.Contains(err.Error(), substr) {
					t.Errorf("error must not contain %q, got: %v", substr, err)
				}
			}
		})
	}
}

func TestE2EStackFlag_RefusedInProduction(t *testing.T) {
	runE2EStackEnvCases(t, []e2eStackEnvCase{
		{
			name:       "flag OFF + APP_ENV=production → no error (dormant)",
			production: true,
		},
		{
			name:            "flag OFF + production + exposed + opt-in → still no error",
			production:      true,
			exposure:        "exposed",
			allowOverTunnel: true,
		},
		{
			name:        "flag ON + production → ERROR",
			enabled:     true,
			production:  true,
			expectError: true,
			errSubstrs:  []string{"refusing to start", "BRIDGE_E2E_STACK", "production"},
		},
		{
			name:            "flag ON + production + tunnel opt-in → ERROR (the opt-in never unlocks production)",
			enabled:         true,
			production:      true,
			exposure:        "exposed",
			allowOverTunnel: true,
			expectError:     true,
			errSubstrs:      []string{"refusing to start", "APP_ENV=production"},
		},
		{
			name:    "flag ON + non-production → no error",
			enabled: true,
		},
	})
}

func TestE2EStackFlag_RefusedOnExposedHostWithoutOptIn(t *testing.T) {
	runE2EStackEnvCases(t, []e2eStackEnvCase{
		{
			name:     "flag OFF + exposed, no opt-in → no error (dormant)",
			exposure: "exposed",
		},
		{
			name:        "flag ON + exposed, no opt-in → ERROR naming the opt-in",
			enabled:     true,
			exposure:    "exposed",
			expectError: true,
			errSubstrs: []string{
				"refusing to start",
				"BRIDGE_HOST_EXPOSURE=exposed",
				"ALLOW_E2E_STACK_OVER_TUNNEL",
			},
			// A boot refusal must not echo anything about the database.
			forbidSubstrs: []string{"postgres"},
		},
		{
			name:            "flag ON + exposed + opt-in → no error (a recorded choice)",
			enabled:         true,
			exposure:        "exposed",
			allowOverTunnel: true,
		},
		{
			name:     "flag ON + exposure unset → no error (default localhost)",
			enabled:  true,
			exposure: "",
		},
		{
			name:     "flag ON + exposure=localhost → no error",
			enabled:  true,
			exposure: "localhost",
		},
		{
			name:        "flag ON + EXPOSED (uppercase) → ERROR",
			enabled:     true,
			exposure:    "EXPOSED",
			expectError: true,
			errSubstrs:  []string{"ALLOW_E2E_STACK_OVER_TUNNEL"},
		},
		{
			name:        "flag ON + Exposed (mixed case) → ERROR",
			enabled:     true,
			exposure:    "Exposed",
			expectError: true,
			errSubstrs:  []string{"ALLOW_E2E_STACK_OVER_TUNNEL"},
		},
		{
			name:        "flag ON + padded ' exposed ' → ERROR (whitespace must not smuggle exposure past the guard)",
			enabled:     true,
			exposure:    "  exposed\t",
			expectError: true,
			errSubstrs:  []string{"ALLOW_E2E_STACK_OVER_TUNNEL"},
		},
		{
			name:        "flag ON + '\\n exposed \\n' → ERROR",
			enabled:     true,
			exposure:    "\n exposed \n",
			expectError: true,
			errSubstrs:  []string{"ALLOW_E2E_STACK_OVER_TUNNEL"},
		},
		{
			name:            "flag ON + padded exposure + opt-in → no error",
			enabled:         true,
			exposure:        " exposed ",
			allowOverTunnel: true,
		},
		{
			name:     "flag ON + exposure=exposed-ish word → no error (only `exposed` declares exposure)",
			enabled:  true,
			exposure: "exposedish",
		},
	})
}

// Plan 094 R2-15: the exposure value is normalized the SAME way in Go,
// Hocuspocus, and the Next.js route. `hostExposureCases` in the shared vector
// is the single source of truth for that normalization; the table above states
// the intent in Go terms, and this test binds it to the file the other two
// implementations assert. A divergence here is exactly the R2-15 defect: two
// services refusing to boot while the internet-facing one keeps serving.
func TestE2EStackFlag_ExposureNormalizationMatchesSharedVector(t *testing.T) {
	path := filepath.Join("..", "..", "..", "scripts", "tests", "e2e-stack-vector.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the shared contract vector must exist at %s: %v", path, err)
	}
	var vector struct {
		Contract struct {
			HostExposure struct {
				Name          string `json:"name"`
				ExposedValue  string `json:"exposedValue"`
				Normalization string `json:"normalization"`
			} `json:"hostExposure"`
			TunnelOptIn struct {
				Name         string `json:"name"`
				EnabledValue string `json:"enabledValue"`
			} `json:"tunnelOptIn"`
		} `json:"contract"`
		HostExposureCases []struct {
			Value   string `json:"value"`
			Exposed bool   `json:"exposed"`
		} `json:"hostExposureCases"`
	}
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(vector.HostExposureCases) == 0 {
		t.Fatal("the shared vector must pin the host-exposure cases")
	}
	if vector.Contract.HostExposure.Name != "BRIDGE_HOST_EXPOSURE" {
		t.Errorf("unexpected exposure variable name %q", vector.Contract.HostExposure.Name)
	}
	if vector.Contract.HostExposure.ExposedValue != "exposed" {
		t.Errorf("unexpected exposed value %q", vector.Contract.HostExposure.ExposedValue)
	}

	for _, row := range vector.HostExposureCases {
		t.Run(fmt.Sprintf("%q", row.Value), func(t *testing.T) {
			// Flag on, not production, no opt-in: the ONLY thing that can
			// refuse is the exposure value.
			err := validateE2EStackEnv(true, false, row.Value, false)
			if row.Exposed && err == nil {
				t.Fatalf("%q is exposed in the shared vector but Go booted without the opt-in", row.Value)
			}
			if !row.Exposed && err != nil {
				t.Fatalf("%q is not exposed in the shared vector but Go refused: %v", row.Value, err)
			}
			if row.Exposed && !strings.Contains(err.Error(), vector.Contract.TunnelOptIn.Name) {
				t.Errorf("the refusal must name %s, got: %v", vector.Contract.TunnelOptIn.Name, err)
			}

			// The opt-in itself is NOT normalized: it must be exactly "true".
			if err := validateE2EStackEnv(true, false, row.Value, true); err != nil {
				t.Errorf("the recorded opt-in must allow %q, got: %v", row.Value, err)
			}
			// And the flag being off keeps every value dormant.
			if err := validateE2EStackEnv(false, false, row.Value, false); err != nil {
				t.Errorf("a disabled flag must not refuse %q, got: %v", row.Value, err)
			}
		})
	}
}
