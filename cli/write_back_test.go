package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/config"
)

// TestSourceCommandsWriteOnlyTheFilesOwnContent: loading applies defaults and
// environment values; add and remove must not write them back (#272).
//
// spec: openspec/specs/runtime-configuration/spec.md#Requirement: Source commands write back only the file's own content
func TestSourceCommandsWriteOnlyTheFilesOwnContent(t *testing.T) {
	const probe = "probe-token-272"
	t.Setenv(config.EnvGitToken, probe)
	t.Setenv("SEMSOURCE_WS_BIND", "127.0.0.1:9999")

	tests := []struct {
		name string
		run  func(term *Term, path string) error
	}{
		{name: "add", run: func(term *Term, path string) error {
			return Add(term, path, []string{"ast", "--path", "./wsB", "--language", "go"})
		}},
		{name: "remove", run: func(term *Term, path string) error { return Remove(term, path, 0) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "semsource.json")
			writeFile(t, path, `{"namespace": "acme", "sources": [{"type": "ast", "path": "./src", "language": "go"}]}`)
			term, _ := newTestTerm("")
			if err := tc.run(term, path); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte(probe)) {
				t.Errorf("written file carries the git token:\n%s", data)
			}
			var doc map[string]json.RawMessage
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			for key := range doc {
				switch key {
				case "namespace", "sources", "config_revision":
				default:
					t.Errorf("written file gained key %q the file did not hold:\n%s", key, data)
				}
			}
		})
	}
}

// TestRemove_InvalidFileIsNotRewritten: a file that fails validation fails the
// command and stays byte for byte as it was.
//
// spec: openspec/specs/runtime-configuration/spec.md#Requirement: Source commands write back only the file's own content
func TestRemove_InvalidFileIsNotRewritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "semsource.json")
	const invalid = `{"namespace": "acme", "git_token": "x", "sources": [{"type": "ast", "path": "./src", "language": "go"}]}`
	writeFile(t, path, invalid)
	term, _ := newTestTerm("")
	if err := Remove(term, path, 0); err == nil {
		t.Fatal("remove on an invalid file succeeded")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != invalid {
		t.Fatalf("invalid file was rewritten:\n%s", data)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
