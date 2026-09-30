//go:build integration

package governance

import (
	"os"
	"testing"

	"github.com/c360studio/semsource/internal/miniotest"
)

// TestMain owns the lazy MinIO fixture used by the governed object-store suites.
func TestMain(m *testing.M) {
	os.Exit(miniotest.RunTests(m))
}
