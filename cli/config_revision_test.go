package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writtenRevision decodes the file as plain JSON, independent of config
// loading: the config_revision it carries (nil when absent) and its source count.
func writtenRevision(t *testing.T, path string) (*int, int) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc struct {
		ConfigRevision *int              `json:"config_revision"`
		Sources        []json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return doc.ConfigRevision, len(doc.Sources)
}

// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: Source commands that write the configuration file advance its revision
func TestSourceCommandsAdvanceConfigRevision(t *testing.T) {
	const twoSources = `[{"type": "ast", "path": "./wsA", "language": "go"}, {"type": "docs", "paths": ["docs/"]}]`
	tests := []struct {
		name         string
		revision     string // the config_revision member, or "" to omit it
		run          func(term *Term, path string) error
		wantErr      bool
		wantRevision *int // nil: the member stays absent
		wantSources  int
	}{
		{
			name: "add to a file without a revision",
			run: func(term *Term, path string) error {
				return Add(term, path, []string{"ast", "--path", "./wsB", "--language", "go"})
			},
			wantRevision: intPtr(1), wantSources: 3,
		},
		{
			name:     "add to a file at revision 5",
			revision: `"config_revision": 5,`,
			run: func(term *Term, path string) error {
				return Add(term, path, []string{"ast", "--path", "./wsB", "--language", "go"})
			},
			wantRevision: intPtr(6), wantSources: 3,
		},
		{
			name:         "remove from a file at revision 2",
			revision:     `"config_revision": 2,`,
			run:          func(term *Term, path string) error { return Remove(term, path, 1) },
			wantRevision: intPtr(3), wantSources: 1,
		},
		{
			name:         "a refused remove writes nothing",
			revision:     `"config_revision": 2,`,
			run:          func(term *Term, path string) error { return Remove(term, path, 9) },
			wantErr:      true,
			wantRevision: intPtr(2), wantSources: 2,
		},
		{
			name: "a refused add writes nothing",
			run: func(term *Term, path string) error {
				return Add(term, path, []string{"not-a-source-type"})
			},
			wantErr:     true,
			wantSources: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "semsource.json")
			doc := `{"namespace": "myorg", ` + tc.revision + `"sources": ` + twoSources + `}`
			if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
				t.Fatal(err)
			}
			term, _ := newTestTerm("")
			err := tc.run(term, path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, tc.wantErr)
			}
			gotRevision, gotSources := writtenRevision(t, path)
			if gotSources != tc.wantSources {
				t.Errorf("sources = %d, want %d", gotSources, tc.wantSources)
			}
			switch {
			case tc.wantRevision == nil && gotRevision != nil:
				t.Errorf("config_revision = %d, want absent", *gotRevision)
			case tc.wantRevision != nil && gotRevision == nil:
				t.Errorf("config_revision absent, want %d", *tc.wantRevision)
			case tc.wantRevision != nil && *gotRevision != *tc.wantRevision:
				t.Errorf("config_revision = %d, want %d", *gotRevision, *tc.wantRevision)
			}
		})
	}
}

func intPtr(v int) *int { return &v }

// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: Source commands that write the configuration file advance its revision
func TestInitCarriesConfigRevisionForward(t *testing.T) {
	detected := &ProjectInfo{Namespace: "acme", HasDocs: true, DocPaths: []string{"docs/"}}
	tests := []struct {
		name         string
		existing     string // the file init overwrites, or "" for none
		wantRevision *int   // nil: the member stays absent
	}{
		{name: "existing file at revision 4", existing: `{"namespace": "old", "config_revision": 4, "sources": []}`, wantRevision: intPtr(5)},
		{name: "existing file without a revision", existing: `{"namespace": "old", "sources": []}`, wantRevision: intPtr(1)},
		{name: "no existing file"},
		{name: "existing file that does not parse", existing: `not json`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "semsource.json")
			input := ""
			if tc.existing != "" {
				if err := os.WriteFile(path, []byte(tc.existing), 0o644); err != nil {
					t.Fatal(err)
				}
				input = "y\n" // confirm the overwrite
			}
			term, _ := newTestTerm(input)
			if err := InitQuick(term, path, detected); err != nil {
				t.Fatalf("InitQuick: %v", err)
			}
			gotRevision, gotSources := writtenRevision(t, path)
			if gotSources != 1 {
				t.Fatalf("sources = %d, want the 1 detected: init did not write the file", gotSources)
			}
			switch {
			case tc.wantRevision == nil && gotRevision != nil:
				t.Errorf("config_revision = %d, want absent", *gotRevision)
			case tc.wantRevision != nil && gotRevision == nil:
				t.Errorf("config_revision absent, want %d", *tc.wantRevision)
			case tc.wantRevision != nil && *gotRevision != *tc.wantRevision:
				t.Errorf("config_revision = %d, want %d", *gotRevision, *tc.wantRevision)
			}
		})
	}
}
