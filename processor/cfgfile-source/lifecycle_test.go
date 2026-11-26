package cfgfilesource

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// Every published cfgfile entity names its root's lifecycle owner and its
// declaring manifest, so a sweep of that root can judge it (#393).
// spec: entity-staleness / Config-file entities go stale with their manifest
func TestCfgfileEveryPublishedEntityCarriesOwnerAndManifestPath(t *testing.T) {
	root := t.TempDir()
	f := newCfgFixture(t, root)
	gomod := writeFile(t, root, "svc/go.mod", twoDeps)
	docker := writeFile(t, root, "img/Dockerfile", "FROM golang:1.23\n")
	f.seed()

	owner := source.EntityLifecycleOwner + "=" + graph.LifecycleOwner("cfgfile-source", root)
	published := f.groupStatements()
	if len(published) != 4 {
		t.Fatalf("published %d entities, want module + 2 deps + image", len(published))
	}
	for subject, stmts := range published {
		if !slices.Contains(stmts, owner) {
			t.Errorf("%s carries no %s", subject, owner)
		}
		want := source.ConfigFilePath + "=" + gomod
		if strings.Contains(subject, ".config.image.") {
			want = source.ConfigFilePath + "=" + docker
		}
		if !slices.Contains(stmts, want) {
			t.Errorf("%s carries no %s: %v", subject, want, stmts)
		}
	}
}

// A version bump republishes the dependency under its existing ID, and the
// reconcile replaces the version fact in place (#393).
// spec: entity-identity-safety / Config-file dependency and image identity excludes the version
func TestCfgfileVersionBumpReconcilesTheDependencyInPlace(t *testing.T) {
	root := t.TempDir()
	f := newCfgFixture(t, root)
	path := writeFile(t, root, "go.mod", "module example.com/svc\n\ngo 1.22\n\nrequire github.com/a/b v1.2.0\n")
	f.seed()
	var depID string
	for subject := range f.groupStatements() {
		if strings.Contains(subject, ".config.dependency.") {
			depID = subject
		}
	}
	if depID == "" {
		t.Fatal("seed published no dependency")
	}

	writeFile(t, root, "go.mod", "module example.com/svc\n\ngo 1.22\n\nrequire github.com/a/b v1.3.0\n")
	f.client.reset()
	f.watchModify(root, path)

	if ids := f.rec.IDs(); !slices.Contains(ids, depID) {
		t.Fatalf("reconciled %v; want the bumped dependency %s reconciled in place", ids, depID)
	}
	var versions []string
	for _, st := range f.groupStatements()[depID] {
		if v, ok := strings.CutPrefix(st, source.ConfigDepVersion+"="); ok {
			versions = append(versions, v)
		}
	}
	if want := []string{"v1.3.0"}; !reflect.DeepEqual(versions, want) {
		t.Errorf("republished versions = %v, want %v", versions, want)
	}
}

// The lifecycle request is scoped by the system the entities carry and owned
// by the absolute root its entities were stamped with (#393).
// spec: entity-staleness / Config-file entities go stale with their manifest
func TestCfgfileLifecycleRequest(t *testing.T) {
	for _, tc := range []struct {
		name, project, instance, wantSystem, wantOwner string
	}{
		// 8b7f48fd82ee = first 12 hex of sha256("/workspace/svc").
		{"path-derived system", "", "", "svc", "cfgfile-source/8b7f48fd82ee"},
		{"project override", "Acme.Core", "cfgfile-source-acme", "acme-core", "cfgfile-source-acme/8b7f48fd82ee"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Component{authority: entityid.Authority{Org: "acme", Platform: "test-a1b2c3"},
				config: Config{Org: "acme", Project: tc.project, InstanceName: tc.instance}}
			got := c.lifecycleRequest("/workspace/svc", graph.LifecycleReasonFileDeleted)
			want := graph.LifecycleRunRequest{
				Org: "acme", Systems: []string{tc.wantSystem}, RootPath: "/workspace/svc",
				Reason: graph.LifecycleReasonFileDeleted, Owner: tc.wantOwner,
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("lifecycleRequest = %+v, want %+v", got, want)
			}
		})
	}
}

// spec: entity-staleness / Config-file entities go stale with their manifest
func TestCfgfileRootsForPath(t *testing.T) {
	c := &Component{config: Config{Paths: []string{"/workspace/svc", "/workspace/web", "/workspace/svc"}}}
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/workspace/svc/go.mod", []string{"/workspace/svc"}},
		{"/workspace/web/sub/package.json", []string{"/workspace/web"}},
		{"/workspace/svc-extra/go.mod", nil},
		{filepath.Join("/elsewhere", "go.mod"), nil},
	} {
		if got := c.rootsForPath(tc.path); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("rootsForPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// A watch:false source is a frozen snapshot that never goes stale, so its
// seed sweeps nothing; a watching source sweeps every root (#393).
// spec: entity-staleness / Config-file entities go stale with their manifest
func TestCfgfileSeedSweepOnlyWhenWatching(t *testing.T) {
	paths := []string{"/workspace/svc", "/workspace/web"}
	frozen := &Component{config: Config{Paths: paths, WatchEnabled: false}}
	if got := frozen.seedSweepRoots(); len(got) != 0 {
		t.Errorf("watch:false seed sweeps %v, want none", got)
	}
	watching := &Component{config: Config{Paths: paths, WatchEnabled: true}}
	if got := watching.seedSweepRoots(); !reflect.DeepEqual(got, paths) {
		t.Errorf("watching seed sweeps %v, want %v", got, paths)
	}
}
