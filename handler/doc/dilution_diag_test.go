package doc_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler/doc"
)

// TestDilutionDiag dumps the real splitter's passages for a file so the
// dilution of a specific answer can be measured offline. Diagnostic only.
//
//	DIAG_FILES=/path/a.md,/path/b.md DIAG_NEEDLE='-p 8083:8083' \
//	  go test ./handler/doc/ -run TestDilutionDiag -v
func TestDilutionDiag(t *testing.T) {
	files := os.Getenv("DIAG_FILES")
	if files == "" {
		t.Skip("set DIAG_FILES")
	}
	needle := os.Getenv("DIAG_NEEDLE")
	out := map[string][]map[string]any{}
	for _, f := range strings.Split(files, ",") {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		ps := doc.SplitPassagesBounded(content, 2000, 400, 6000)
		var rows []map[string]any
		for _, p := range ps {
			hit := needle != "" && strings.Contains(p.Body, needle)
			rows = append(rows, map[string]any{
				"heading": p.Heading, "bytes": len(p.Body), "needle": hit, "body": p.Body,
			})
		}
		out[f] = rows
	}
	b, _ := json.Marshal(out)
	os.WriteFile(os.Getenv("DIAG_OUT"), b, 0o644)
	t.Logf("wrote %s", os.Getenv("DIAG_OUT"))
}
