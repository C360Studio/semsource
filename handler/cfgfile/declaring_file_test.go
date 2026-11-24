package cfgfile_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/c360studio/semsource/handler"
	cfgfile "github.com/c360studio/semsource/handler/cfgfile"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// writeTree writes files (slash-separated path relative to root → content).
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

func ingestTree(t *testing.T, root string) []*handler.EntityState {
	t.Helper()
	states, err := cfgfile.New(nil).IngestEntityStates(context.Background(),
		&stubSourceConfig{sourceType: "config", path: root}, testAuthority("acme"))
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}
	return states
}

// statesTitled returns the states of entityType whose dc.terms.title is title,
// keyed by ID. Two states under one ID collapse to one key: that is the defect.
func statesTitled(states []*handler.EntityState, entityType, title string) map[string]*handler.EntityState {
	out := map[string]*handler.EntityState{}
	for _, s := range collectByType(states, entityType) {
		if tripleObject(s, source.DcTitle) == title {
			out[s.ID] = s
		}
	}
	return out
}

// spec: entity-identity-safety / Config-file entities are keyed by their declaring file
func TestIngestEntityStates_SharedCoordinateGetsOneIDPerManifest(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a/pom.xml": `<project><groupId>com.acme</groupId><artifactId>a</artifactId><version>1</version>
<dependencies><dependency><groupId>org.slf4j</groupId><artifactId>slf4j-api</artifactId><version>2.0.9</version><scope>test</scope></dependency></dependencies></project>`,
		"b/build.gradle": "dependencies {\n    implementation 'org.slf4j:slf4j-api:2.0.9'\n}\n",
		"c/go.mod":       "module example.com/c\n\ngo 1.23\n\nrequire golang.org/x/text v0.14.0 // indirect\n",
		"d/go.mod":       "module example.com/d\n\ngo 1.23\n\nrequire golang.org/x/text v0.14.0\n",
	})
	states := ingestTree(t, root)

	deps := statesTitled(states, "dependency", "org.slf4j:slf4j-api")
	if len(deps) != 2 {
		t.Fatalf("want 2 slf4j dependency IDs (one per manifest), got %d", len(deps))
	}
	var scoped, configured int
	for id, s := range deps {
		scope := tripleObject(s, source.ConfigDepScope)
		conf := tripleObject(s, source.ConfigDepConfiguration)
		if scope != "" && conf != "" {
			t.Errorf("%s carries both a Maven scope and a Gradle configuration", id)
		}
		if scope == "test" {
			scoped++
		}
		if conf == "implementation" {
			configured++
		}
	}
	if scoped != 1 || configured != 1 {
		t.Errorf("want one test-scoped and one implementation-configured entity, got %d and %d", scoped, configured)
	}

	if got := len(statesTitled(states, "dependency", "golang.org/x/text")); got != 2 {
		t.Errorf("want 2 golang.org/x/text dependency IDs (indirect in c, direct in d), got %d", got)
	}
}

// spec: entity-identity-safety / Config-file entities are keyed by their declaring file
func TestIngestEntityStates_SameNamedGradleDirsDoNotCollide(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a/core/build.gradle": "dependencies {\n}\n",
		"b/core/build.gradle": "dependencies {\n}\n",
	})
	if got := len(statesTitled(ingestTree(t, root), "project", "core")); got != 2 {
		t.Fatalf("want 2 Gradle project IDs for a/core and b/core, got %d", got)
	}
}

// spec: entity-identity-safety / Config-file entities are keyed by their declaring file
func TestIngestEntityStates_DockerfilesSharingABaseImageKeepTheirPorts(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"api/Dockerfile":    "FROM golang:1.23\nEXPOSE 8080\n",
		"worker/Dockerfile": "FROM golang:1.23\nEXPOSE 9090\n",
	})
	images := statesTitled(ingestTree(t, root), "image", "golang:1.23")
	if len(images) != 2 {
		t.Fatalf("want 2 image IDs (one per Dockerfile), got %d", len(images))
	}
	var ports []string
	for id, s := range images {
		p := collectTriplesByPred([]*handler.EntityState{s}, source.ConfigImagePorts)
		if len(p) != 1 {
			t.Errorf("%s: want exactly its own file's one port, got %v", id, p)
		}
		ports = append(ports, p...)
	}
	sort.Strings(ports)
	if len(ports) != 2 || ports[0] != "8080" || ports[1] != "9090" {
		t.Errorf("want ports [8080 9090] across the two images, got %v", ports)
	}
}

// spec: entity-identity-safety / Config-file entities are keyed by their declaring file
func TestIngestEntityStates_IDsDoNotDependOnCheckoutLocation(t *testing.T) {
	// Same root basename (the system slug), different absolute locations.
	// The nameless package.json and the Gradle project exercise the
	// formerly path-hashed fallbacks.
	files := map[string]string{
		"web/package.json":     `{"version":"1.0.0","dependencies":{"left-pad":"1.3.0"}}`,
		"svc/build.gradle":     "dependencies {\n    implementation 'junit:junit:4.13'\n}\n",
		"pom.xml":              `<project><version>1</version></project>`,
		"tools/gen/Dockerfile": "FROM alpine:3.20\n",
		"mod/go.mod":           "module example.com/mod\n\ngo 1.23\n",
	}
	ids := func(parent string) []string {
		root := filepath.Join(parent, "repo")
		writeTree(t, root, files)
		var out []string
		for _, s := range ingestTree(t, root) {
			out = append(out, s.ID)
		}
		sort.Strings(out)
		return out
	}
	first, second := ids(filepath.Join(t.TempDir(), "x")), ids(filepath.Join(t.TempDir(), "y", "z"))
	if len(first) == 0 {
		t.Fatal("no entities ingested")
	}
	if len(first) != len(second) {
		t.Fatalf("entity counts differ across checkouts: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("ID differs across checkouts: %s vs %s", first[i], second[i])
		}
	}
}
