package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestValidateStorageConfig(t *testing.T) {
	tests := []struct {
		name         string
		env          map[string]string
		wantErr      bool
		errSubstring string
	}{
		{
			name: "missing database url and missing memory store opt-in",
			env: map[string]string{
				"DATABASE_URL":       "",
				"ALLOW_MEMORY_STORE": "false",
				"ALLOW_DEV_HEADERS":  "false",
				"ENVIRONMENT":        "",
			},
			wantErr:      true,
			errSubstring: "DATABASE_URL is required unless ALLOW_MEMORY_STORE=true",
		},
		{
			name: "allow memory store without dev headers flag is rejected",
			env: map[string]string{
				"DATABASE_URL":       "",
				"ALLOW_MEMORY_STORE": "true",
				"ALLOW_DEV_HEADERS":  "false",
				"ENVIRONMENT":        "",
			},
			wantErr:      true,
			errSubstring: "ALLOW_MEMORY_STORE=true is prohibited in production configuration",
		},
		{
			name: "allow memory store with dev headers unset is rejected",
			env: map[string]string{
				"DATABASE_URL":       "",
				"ALLOW_MEMORY_STORE": "true",
				"ALLOW_DEV_HEADERS":  "",
				"ENVIRONMENT":        "",
			},
			wantErr:      true,
			errSubstring: "ALLOW_MEMORY_STORE=true is prohibited in production configuration",
		},
		{
			name: "allow memory store with dev headers true but ENVIRONMENT=production is rejected",
			env: map[string]string{
				"DATABASE_URL":       "",
				"ALLOW_MEMORY_STORE": "true",
				"ALLOW_DEV_HEADERS":  "true",
				"ENVIRONMENT":        "production",
			},
			wantErr:      true,
			errSubstring: "ALLOW_MEMORY_STORE=true is prohibited in production configuration",
		},
		{
			name: "allow memory store with dev headers true but APP_ENV=prod is rejected",
			env: map[string]string{
				"DATABASE_URL":       "",
				"ALLOW_MEMORY_STORE": "true",
				"ALLOW_DEV_HEADERS":  "true",
				"APP_ENV":            "prod",
			},
			wantErr:      true,
			errSubstring: "ALLOW_MEMORY_STORE=true is prohibited in production configuration",
		},
		{
			name: "allow memory store with dev headers true in non-production is accepted",
			env: map[string]string{
				"DATABASE_URL":       "",
				"ALLOW_MEMORY_STORE": "true",
				"ALLOW_DEV_HEADERS":  "true",
				"ENVIRONMENT":        "development",
			},
			wantErr: false,
		},
		{
			name: "valid database url without memory store is accepted",
			env: map[string]string{
				"DATABASE_URL":       "postgresql://user:pass@localhost:5433/db",
				"ALLOW_MEMORY_STORE": "false",
				"ALLOW_DEV_HEADERS":  "false",
				"ENVIRONMENT":        "production",
			},
			wantErr: false,
		},
		{
			name: "database url set but memory store true in production is rejected",
			env: map[string]string{
				"DATABASE_URL":       "postgresql://user:pass@localhost:5433/db",
				"ALLOW_MEMORY_STORE": "true",
				"ALLOW_DEV_HEADERS":  "false",
				"ENVIRONMENT":        "production",
			},
			wantErr:      true,
			errSubstring: "ALLOW_MEMORY_STORE=true is prohibited in production configuration",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Clear relevant env vars
			t.Setenv("DATABASE_URL", "")
			t.Setenv("ALLOW_MEMORY_STORE", "")
			t.Setenv("ALLOW_DEV_HEADERS", "")
			t.Setenv("ENVIRONMENT", "")
			t.Setenv("APP_ENV", "")
			t.Setenv("NVUTO_ENV", "")

			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			err := validateStorageConfig()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errSubstring != "" && !strings.Contains(err.Error(), tc.errSubstring) {
					t.Fatalf("expected error containing %q, got %q", tc.errSubstring, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestHelperProcessMain is invoked as a subprocess to test fatal os.Exit(1) on invalid startup config.
func TestHelperProcessMain(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	main()
}

func TestMain_FatalStartupOnMemoryStoreInProduction(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessMain")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		// Set valid auth so auth validation passes
		"ALLOW_HMAC_AUTH=true",
		"JWT_HMAC_SECRET=0123456789abcdef0123456789abcdef",
		// Production storage violation: ALLOW_MEMORY_STORE=true without ALLOW_DEV_HEADERS
		"DATABASE_URL=",
		"ALLOW_MEMORY_STORE=true",
		"ALLOW_DEV_HEADERS=false",
		"ENVIRONMENT=production",
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected server process to exit with fatal error, but it exited successfully. Output:\n%s", out.String())
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
	}

	output := out.String()
	if !strings.Contains(output, "storage_configuration_invalid") && !strings.Contains(output, "prohibited in production") {
		t.Fatalf("expected output to mention storage configuration failure, got:\n%s", output)
	}
}
