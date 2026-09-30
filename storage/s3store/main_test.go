//go:build integration || garage

package s3store_test

import (
	"os"
	"testing"

	"github.com/c360studio/semsource/internal/garagetest"
	"github.com/c360studio/semsource/internal/miniotest"
)

// TestMain tears down whichever servers the run actually started.
//
// It lives here rather than beside either suite because the two build tags
// compose: `-tags=integration,garage` runs the MinIO and Garage suites in one
// process, and a TestMain in each file would not compile together. Both
// providers start lazily, so naming both here costs a run nothing for the one
// it did not use.
func TestMain(m *testing.M) {
	os.Exit(miniotest.RunTests(m, garagetest.Terminate))
}
