package sourcemanifest

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
)

func TestDesiredRemovalRetainsSourceIdentityAcrossPersistence(t *testing.T) {
	cases := []config.SourceEntry{
		{Type: "ast", Path: "/tmp/project", Project: "my-project", Version: "v1", BranchSlug: "topic", InstanceSuffix: "parent", Languages: []string{"go", "typescript"}},
		{Type: "docs", Paths: []string{"/tmp/docs"}, Project: "Manuals", BranchSlug: "topic", InstanceSuffix: "parent"},
		{Type: "config", Paths: []string{"/tmp/config"}, Project: "Builds", BranchSlug: "topic", InstanceSuffix: "parent"},
		{Type: "s3", Bucket: "reports", Prefix: "monthly/", Project: "financial", Version: "v1", Endpoint: "https://storage.example", Region: "test", PathStyle: true},
	}
	for _, source := range cases {
		t.Run(source.Type, func(t *testing.T) {
			store := newDesiredStore(t)
			component := &Component{logger: slog.Default()}
			cfg := IngestHandlerConfig{Namespace: "acme", Store: store, Spawn: sourcespawn.Options{Org: "acme"}}
			added := component.addSource(context.Background(), AddRequest{Source: source}, cfg)
			if added.Error != nil || len(added.Components) != 1 {
				t.Fatalf("add=%+v", added)
			}
			handle := added.Components[0].InstanceName
			store.failManifest = true
			partial := component.removeSource(context.Background(), handle, "test", cfg)
			if partial.Error == nil || !partial.DesiredChanged || partial.RuntimeChanged {
				t.Fatalf("partial receipt=%+v", partial)
			}
			store.failManifest = false
			retried := component.removeSource(context.Background(), handle, "test", cfg)
			if retried.Error != nil || !retried.RestartRequired || retried.RuntimeChanged {
				t.Fatalf("retry=%+v", retried)
			}
			var desired Config
			if err := json.Unmarshal(store.components["source-manifest"].Config, &desired); err != nil {
				t.Fatal(err)
			}
			if len(desired.Sources) != 0 || desired.ExpectedSourceCount != 0 {
				t.Fatalf("removed %s but next boot advertises %+v", handle, desired)
			}
		})
	}
}

func TestManifestKeepsDistinctVersionsAndObjectPrefixes(t *testing.T) {
	component := &Component{}
	for _, src := range []config.SourceEntry{
		{Type: "ast", Path: "/src", Project: "library", Version: "v1"},
		{Type: "ast", Path: "/src", Project: "library", Version: "v2"},
		{Type: "s3", Bucket: "reports", Prefix: "monthly"},
		{Type: "s3", Bucket: "reports", Prefix: "yearly"},
	} {
		component.appendManifestSources(src)
	}
	if len(component.manifestSources) != 4 {
		t.Fatalf("distinct sources collapsed: %+v", component.manifestSources)
	}
}

func TestDesiredObjectStoreRefreshReplacesMetadataForSameHandle(t *testing.T) {
	store := newDesiredStore(t)
	component := &Component{logger: slog.Default()}
	cfg := IngestHandlerConfig{Namespace: "acme", Store: store, Spawn: sourcespawn.Options{Org: "acme"}}
	first := config.SourceEntry{Type: "s3", Bucket: "reports", Prefix: "monthly", Project: "old", Version: "v1"}
	second := first
	second.Project, second.Version, second.Watch = "new", "v2", true
	added := component.addSource(context.Background(), AddRequest{Source: first}, cfg)
	updated := component.addSource(context.Background(), AddRequest{Source: second}, cfg)
	if added.Error != nil || updated.Error != nil {
		t.Fatalf("add/refresh errors: %v / %v", added.Error, updated.Error)
	}
	if added.Components[0].InstanceName != updated.Components[0].InstanceName {
		t.Fatal("object-store refresh changed handle")
	}
	var desired Config
	if err := json.Unmarshal(store.components["source-manifest"].Config, &desired); err != nil {
		t.Fatal(err)
	}
	if len(desired.Sources) != 1 || desired.Sources[0].Version != "v2" || desired.Sources[0].Project != "new" || !desired.Sources[0].Watch {
		t.Fatalf("refresh left stale or duplicate desired description: %+v", desired)
	}
}

func TestDesiredRefreshRemovalUsesComponentHandle(t *testing.T) {
	cases := []config.SourceEntry{
		{Type: "url", URLs: []string{"https://example.com/docs"}},
		{Type: "docs", Paths: []string{"/tmp/docs"}},
		{Type: "config", Paths: []string{"/tmp/config"}},
		{Type: "image", Paths: []string{"/tmp/images"}},
		{Type: "audio", Paths: []string{"/tmp/audio"}},
		{Type: "video", Paths: []string{"/tmp/video"}},
		{Type: "git", Path: "/tmp/repo", Branch: "main"},
	}
	for _, first := range cases {
		t.Run(first.Type, func(t *testing.T) {
			store := newDesiredStore(t)
			c := &Component{logger: slog.Default()}
			cfg := IngestHandlerConfig{Namespace: "acme", Store: store, Spawn: sourcespawn.Options{Org: "acme"}}
			next := first
			if len(first.URLs) > 0 {
				next.URLs = append(append([]string(nil), first.URLs...), "https://example.com/api")
			}
			if len(first.Paths) > 0 {
				next.Paths = append(append([]string(nil), first.Paths...), "/tmp/extra")
			}
			if first.Type != "docs" && first.Type != "config" {
				next.Project = "updated-metadata"
			}
			added := c.addSource(context.Background(), AddRequest{Source: first}, cfg)
			updated := c.addSource(context.Background(), AddRequest{Source: next}, cfg)
			if added.Error != nil || updated.Error != nil {
				t.Fatalf("add/refresh: %+v %+v", added, updated)
			}
			if added.Components[0].InstanceName != updated.Components[0].InstanceName {
				t.Fatal("refresh changed handle")
			}
			var desired Config
			if err := json.Unmarshal(store.components["source-manifest"].Config, &desired); err != nil {
				t.Fatal(err)
			}
			if len(desired.Sources) != 1 {
				t.Fatalf("refresh duplicated next-boot descriptor: %+v", desired.Sources)
			}
			removed := c.removeSource(context.Background(), added.Components[0].InstanceName, "test", cfg)
			if removed.Error != nil {
				t.Fatal(removed.Error)
			}
			if err := json.Unmarshal(store.components["source-manifest"].Config, &desired); err != nil {
				t.Fatal(err)
			}
			if len(desired.Sources) != 0 || desired.ExpectedSourceCount != 0 {
				t.Fatalf("removed sole component but next-boot manifest retains %+v", desired)
			}
		})
	}
}

func TestManifestRemovalReconcilesEveryOwningDescription(t *testing.T) {
	opts := sourcespawn.Options{Org: "acme", WorkspaceDir: "/tmp/work"}
	repo := config.SourceEntry{Type: "repo", Path: "/tmp/repo", Branch: "main"}
	ast := config.SourceEntry{Type: "ast", Path: "/tmp/repo"}
	store := newDesiredStore(t)
	built, err := sourcespawn.Build(repo, opts)
	if err != nil {
		t.Fatal(err)
	}
	for name, cfg := range built {
		store.components[name] = cfg
	}
	handles, err := sourcespawn.InstanceNames(ast, opts)
	if err != nil {
		t.Fatal(err)
	}
	var handle string
	for name := range handles {
		handle = name
	}
	delete(store.components, handle)
	c := &Component{manifestSources: []ManifestSource{sourceEntryToManifestSource(repo), sourceEntryToManifestSource(ast), sourceEntryToManifestSource(ast)}}
	if !c.removeManifestSourceByInstance(handle, opts, store) {
		t.Fatal("explicit descriptions not removed")
	}
	if len(c.manifestSources) != 1 || c.manifestSources[0].Type != "repo" {
		t.Fatalf("must retain only repo with surviving siblings: %+v", c.manifestSources)
	}
}
