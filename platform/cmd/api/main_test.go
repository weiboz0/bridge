package main

import (
	"strings"
	"testing"
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
