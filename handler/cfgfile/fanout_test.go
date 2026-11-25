package cfgfile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
)

// statementSet renders states as sorted "id predicate object" lines, the
// parts of a publish the #336 reconcile converges on.
func statementSet(states []*handler.EntityState) []string {
	var out []string
	for _, s := range states {
		for _, tr := range s.Triples {
			out = append(out, fmt.Sprintf("%s %s %v", s.ID, tr.Predicate, tr.Object))
		}
	}
	slices.Sort(out)
	return out
}

// The watcher's change event for a manifest carries exactly what a full
// ingest builds for that manifest, so a watch republish is as complete as a
// seed — the condition cfgfile-source's reconcile opt-in rests on (#384).
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestFanOutCarriesTheManifestsCompleteStates(t *testing.T) {
	manifests := map[string]string{
		"go.mod":       "module example.com/svc\n\ngo 1.22\n\nrequire github.com/a/one v1.0.0\n",
		"package.json": `{"name":"web","version":"1.0.0","dependencies":{"lodash":"^4"},"devDependencies":{"lodash":"^4"}}`,
		"Dockerfile":   "FROM golang:1.22 AS build\nFROM alpine:3\nEXPOSE 8080\n",
		"pom.xml":      `<project><groupId>g</groupId><artifactId>app</artifactId><version>1</version><modules><module>core</module></modules></project>`,
		"build.gradle": "dependencies {\n  implementation 'org.x:lib:1.0'\n  testImplementation 'org.x:lib:1.0'\n}\n",
	}
	authority := entityid.Authority{Org: "acme", Platform: "test-a1b2c3"}
	for name, content := range manifests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "svc", name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			h := New(&Config{Authority: authority})
			seeded, err := h.IngestEntityStates(context.Background(), stubCfg{root: root}, authority)
			if err != nil {
				t.Fatal(err)
			}

			in := make(chan handler.ChangeEvent, 1)
			out := make(chan handler.ChangeEvent, 1)
			in <- handler.ChangeEvent{Path: path, Operation: handler.OperationModify}
			close(in)
			h.fanOut(context.Background(), root, in, out)
			if len(out) != 1 {
				t.Fatalf("fanOut forwarded %d events, want 1", len(out))
			}
			watched := (<-out).EntityStates

			want, got := statementSet(seeded), statementSet(watched)
			if len(want) == 0 {
				t.Fatal("seed built nothing; the fixture is vacuous")
			}
			if !slices.Equal(got, want) {
				t.Errorf("watch statements differ from the seed's\n got: %v\nwant: %v", got, want)
			}
		})
	}
}

// stubCfg is a handler.SourceConfig over one root. The embedded interface
// is nil, so a method the ingest does not need panics loudly if called.
type stubCfg struct {
	handler.SourceConfig
	root string
}

func (s stubCfg) GetType() string    { return handler.SourceTypeConfig }
func (s stubCfg) GetPath() string    { return s.root }
func (s stubCfg) GetPaths() []string { return []string{s.root} }
