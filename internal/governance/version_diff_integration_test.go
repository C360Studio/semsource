//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/processor/supersession"
	semsourceast "github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/ontology"
	"github.com/c360studio/semstreams/component"
	queryclient "github.com/c360studio/semstreams/graph/query"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/storage/objectstore"
)

// TestIntegration_VersionDiff drives the version-diff query end to end over a real
// graph-ingest/index/query stack + a real ObjectStore: it ingests a project at two
// versions with a changed symbol, an unchanged symbol, an added-in-to symbol, and a
// removed-in-to symbol, then calls graph.query.versionDiff and asserts the
// classification, counts, and verbatim before/after bodies — plus the honest
// missing-version envelope.
func TestIntegration_VersionDiff(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "GRAPH",
			Subjects: []string{"graph.ingest.entity"},
		}),
	)
	if _, err := BootstrapStandalone(ctx, tc.Client, nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}

	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	mr := metric.NewMetricsRegistry()

	ingest := startGraphIngest(t, ctx, tc.Client, reg, mr)
	t.Cleanup(func() { _ = ingest.Stop(5 * time.Second) })
	index := startGraphIndex(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = index.Stop(5 * time.Second) })
	q := startGraphQuery(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = q.Stop(5 * time.Second) })

	// Offload verbatim bodies for the changed symbol (before/after) to the real
	// ObjectStore the diff's fallback resolver attaches to (CONTENT bucket).
	store, err := objectstore.NewStoreWithConfig(ctx, tc.Client, objectstore.Config{
		BucketName:   semsourcegraph.BodyStoreBucket,
		InstanceName: semsourcegraph.BodyStoreInstance,
	})
	if err != nil {
		t.Fatalf("objectstore: %v", err)
	}
	const editedBefore = "func Edited() { return 1 }"
	const editedAfter = "func Edited() { return 2 }"
	if err := store.Put(ctx, "code:edited-v1", []byte(editedBefore)); err != nil {
		t.Fatalf("put v1 body: %v", err)
	}
	if err := store.Put(ctx, "code:edited-v2", []byte(editedAfter)); err != nil {
		t.Fatalf("put v2 body: %v", err)
	}

	pub, err := entitypub.New(tc.Client, nil)
	if err != nil {
		t.Fatalf("entitypub.New: %v", err)
	}
	pub.Start(ctx)
	const proj = "semstreams"
	ids := []string{
		publishVersionedBody(t, ctx, pub, proj, "v1.9.0", "pkg/edited.go", "Edited", "pkg", "code:edited-v1"),
		publishVersionedBody(t, ctx, pub, proj, "v1.10.0", "pkg/edited.go", "Edited", "pkg", "code:edited-v2"),
		publishVersionedBody(t, ctx, pub, proj, "v1.9.0", "pkg/stable.go", "Stable", "pkg", "code:stable-same"),
		publishVersionedBody(t, ctx, pub, proj, "v1.10.0", "pkg/stable.go", "Stable", "pkg", "code:stable-same"),
		publishVersionedBody(t, ctx, pub, proj, "v1.9.0", "pkg/gone.go", "Gone", "pkg", "code:gone"),
		publishVersionedBody(t, ctx, pub, proj, "v1.10.0", "pkg/new.go", "New", "pkg", "code:new"),
	}
	pub.Stop()

	qc, err := queryclient.NewClient(ctx, tc.Client, nil)
	if err != nil {
		t.Fatalf("query client: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })
	for _, id := range ids {
		if _, ok := waitEntity(t, ctx, qc, id, 20*time.Second); !ok {
			t.Fatalf("entity never became queryable: %s", id)
		}
	}

	// Start the supersession component (serves graph.query.versionDiff). No
	// StoreRegistry in deps, so the diff's body resolver falls back to attaching
	// the CONTENT objectstore — the store we offloaded bodies to above.
	scfg, _ := json.Marshal(map[string]any{"max_entities": 1000})
	sdisc, err := supersession.NewComponent(scfg, component.Dependencies{NATSClient: tc.Client})
	if err != nil {
		t.Fatalf("supersession NewComponent: %v", err)
	}
	scomp := sdisc.(*supersession.Component)
	if err := scomp.Start(ctx); err != nil {
		t.Fatalf("supersession Start: %v", err)
	}
	t.Cleanup(func() { _ = scomp.Stop(5 * time.Second) })

	// The changeset over v1.9.0 → v1.10.0.
	resp := requestDiff(t, ctx, tc.Client, proj, "v1.9.0", "v1.10.0")
	if resp.Counts.Added != 1 || resp.Counts.Removed != 1 || resp.Counts.Changed != 1 || resp.Counts.Unchanged != 1 {
		t.Fatalf("counts = %+v; want added1 removed1 changed1 unchanged1", resp.Counts)
	}
	byName := map[string]supersession.Change{}
	for _, ch := range resp.Changes {
		byName[ch.Name] = ch
	}
	if byName["New"].Status != "added" {
		t.Errorf("New: %+v", byName["New"])
	}
	if byName["Gone"].Status != "removed" {
		t.Errorf("Gone: %+v", byName["Gone"])
	}
	edited := byName["Edited"]
	if edited.Status != "changed" || edited.FromBody != editedBefore || edited.ToBody != editedAfter {
		t.Errorf("Edited should be changed with before/after bodies; got status=%q from=%q to=%q",
			edited.Status, edited.FromBody, edited.ToBody)
	}
	if _, listed := byName["Stable"]; listed {
		t.Errorf("Stable (unchanged) must not be listed: %+v", byName["Stable"])
	}

	// Honest missing-version envelope: an unknown `to` is not a giant "removed" diff.
	missing := requestDiff(t, ctx, tc.Client, proj, "v1.9.0", "v99.0.0")
	if len(missing.Changes) != 0 || missing.Note == "" {
		t.Errorf("unknown version should return empty changes + a note; got %d changes, note=%q",
			len(missing.Changes), missing.Note)
	}
}

// publishVersionedBody publishes a versioned Go function entity that also carries
// the code.body.store handle (so the version diff can hydrate its verbatim body),
// and returns its ID.
func publishVersionedBody(t *testing.T, ctx context.Context, pub *entitypub.Publisher,
	project, version, path, name, pkg, bodyKey string) string {
	t.Helper()
	const org, lang, ctype = "acme", "golang", "function"
	system := entityid.ScopedSystemSlug(project, version)
	inst := semsourceast.BuildInstanceID(path, name, semsourceast.TypeFunction)
	id := entityid.Build(org, entityid.PlatformSemsource, lang, system, ctype, inst)
	now := time.Now()
	triples := []message.Triple{
		{Subject: id, Predicate: semsourceast.CodeType, Object: ctype},
		{Subject: id, Predicate: semsourceast.DcTitle, Object: name},
		{Subject: id, Predicate: semsourceast.CodePath, Object: path},
		{Subject: id, Predicate: semsourceast.CodePackage, Object: pkg},
		{Subject: id, Predicate: semsourceast.CodeProject, Object: project},
		{Subject: id, Predicate: semsourceast.CodeVersion, Object: version},
		{Subject: id, Predicate: semsourceast.CodeBodyKey, Object: bodyKey},
		{Subject: id, Predicate: semsourceast.CodeBodyStore, Object: semsourcegraph.BodyStoreInstance},
		{Subject: id, Predicate: semsourceast.DcCreated, Object: now.Format(time.RFC3339)},
	}
	payload := &semsourcegraph.EntityPayload{
		ID:                  id,
		TripleData:          ontology.StampClass(id, triples, now),
		UpdatedAt:           now,
		IndexingProfileHint: semsourcegraph.IndexingProfileContent,
	}
	if err := entitypub.ValidatePayload(payload); err != nil {
		t.Fatalf("invalid versioned payload %s: %v", id, err)
	}
	pub.Send(payload)
	return id
}

// requestDiff calls graph.query.versionDiff and decodes the response.
func requestDiff(t *testing.T, ctx context.Context, client *natsclient.Client, project, from, to string) supersession.VersionDiffResponse {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"project": project, "from": from, "to": to})
	raw, err := client.Request(ctx, "graph.query.versionDiff", body, 20*time.Second)
	if err != nil {
		t.Fatalf("versionDiff request: %v", err)
	}
	var resp supersession.VersionDiffResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode versionDiff %q: %v", raw, err)
	}
	return resp
}
