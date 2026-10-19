package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// retryTransient retries YSQL serialization (40001) and deadlock (40P01)
// aborts, which clients are expected to retry under snapshot isolation.
func retryTransient[T any](op func() (T, error)) (T, error) {
	for attempt := 0; ; attempt++ {
		value, err := op()
		var pe *pgconn.PgError
		if err == nil || !errors.As(err, &pe) || (pe.Code != "40001" && pe.Code != "40P01") || attempt >= 20 {
			return value, err
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
}

// integrationDSN returns the YSQL test DSN. TEST_DATABASE_URL is the shared
// contract used by every integration package; DATABASE_URL is still accepted
// for this package's existing invocations. With NVUTO_REQUIRE_INTEGRATION=true
// a missing DSN fails the test, so a required CI job cannot pass by skipping.
func integrationDSN(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		if dsn := os.Getenv(name); dsn != "" {
			return dsn
		}
	}
	if strings.EqualFold(os.Getenv("NVUTO_REQUIRE_INTEGRATION"), "true") {
		t.Fatal("TEST_DATABASE_URL (or DATABASE_URL) is required when NVUTO_REQUIRE_INTEGRATION=true")
	}
	t.Skip("TEST_DATABASE_URL / DATABASE_URL not configured")
	return ""
}

// runID is unique per test run, so global primary keys never collide when the
// suite is re-run against the same database.
func runID(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(buf))
}
