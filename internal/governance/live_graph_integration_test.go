//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/binaryproof"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semsource/storage/filestore"
	"github.com/c360studio/semstreams/component"
	semgraph "github.com/c360studio/semstreams/graph"
	graphqueryclient "github.com/c360studio/semstreams/graph/query"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/ownership"
	graphindex "github.com/c360studio/semstreams/processor/graph-index"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	graphquery "github.com/c360studio/semstreams/processor/graph-query"
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

func TestIntegration_GovernedGraphIngestStoresSemsourceEntity(t *testing.T) {
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
	metricsRegistry := metric.NewMetricsRegistry()

	configJSON, err := json.Marshal(map[string]any{
		"enforce_owner_lease": false,
		"ports": map[string]any{
			"inputs": []map[string]any{
				{
					"name":        "entity_stream",
					"type":        "jetstream",
					"subject":     "graph.ingest.entity",
					"stream_name": "GRAPH",
					"config":      map[string]any{"deliver_policy": "all"},
				},
			},
			"outputs": []map[string]any{
				{"name": "entity_states", "type": "kv-write", "subject": "ENTITY_STATES"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal graph-ingest config: %v", err)
	}

	discovered, err := graphingest.CreateGraphIngest(configJSON, component.Dependencies{
		NATSClient:      tc.Client,
		PayloadRegistry: reg,
		MetricsRegistry: metricsRegistry,
	})
	if err != nil {
		t.Fatalf("CreateGraphIngest() error = %v", err)
	}
	ingest := discovered.(*graphingest.Component)
	if err := ingest.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if err := ingest.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		_ = ingest.Stop(5 * time.Second)
	})

	cases := []struct {
		name      string
		entityID  string
		profile   string
		predicate string
		object    any
		storage   *message.StorageReference
	}{
		{
			name:      "doc",
			entityID:  "acme.semsource.web.docs.doc.abc123",
			profile:   semsourcegraph.IndexingProfileContent,
			predicate: source.DocContent,
			object:    "hello graph",
		},
		{
			name:      "url",
			entityID:  "acme.semsource.web.example-com.page.url123",
			profile:   semsourcegraph.IndexingProfileContent,
			predicate: source.WebURL,
			object:    "https://example.com/spec",
		},
		{
			name:      "git",
			entityID:  "acme.semsource.git.repo.commit.abcdef0",
			profile:   semsourcegraph.IndexingProfileContent,
			predicate: source.GitCommitSubject,
			object:    "feat: add governed graph",
		},
		{
			name:      "config",
			entityID:  "acme.semsource.config.repo.gomod.app",
			profile:   semsourcegraph.IndexingProfileControl,
			predicate: source.ConfigModulePath,
			object:    "github.com/acme/app",
		},
		{
			name:      "media",
			entityID:  "acme.semsource.media.assets.image.img123",
			profile:   semsourcegraph.IndexingProfileControl,
			predicate: source.MediaStorageRef,
			object:    "media/assets/img123",
			storage: &message.StorageReference{
				StorageInstance: "semsource-media",
				Key:             "media/assets/img123",
				ContentType:     "image/png",
				Size:            42,
			},
		},
		{
			name:      "trace",
			entityID:  "acme.semsource.media.assets.keyframe.kf001",
			profile:   semsourcegraph.IndexingProfileTrace,
			predicate: source.MediaKeyframeOf,
			object:    "acme.semsource.media.assets.video.vid001",
		},
	}

	for _, tcCase := range cases {
		t.Run(tcCase.name, func(t *testing.T) {
			publishSemsourceEntity(t, ctx, tc.Client, tcCase.entityID, tcCase.profile, []message.Triple{
				{Subject: tcCase.entityID, Predicate: tcCase.predicate, Object: tcCase.object},
			}, tcCase.storage)

			stored := waitForEntityState(t, ctx, tc.Client, tcCase.entityID, 5*time.Second)
			if stored.ID != tcCase.entityID {
				t.Fatalf("stored ID = %q, want %q", stored.ID, tcCase.entityID)
			}
			if stored.MessageType.Key() != semsourcegraph.EntityType.Key() {
				t.Fatalf("stored MessageType = %q, want %q", stored.MessageType.Key(), semsourcegraph.EntityType.Key())
			}
			if got := profileValues(stored); len(got) != 1 || got[0] != tcCase.profile {
				t.Fatalf("indexing profile values = %v, want [%s]", got, tcCase.profile)
			}
			if !hasPredicate(stored, tcCase.predicate) {
				t.Fatalf("stored entity missing predicate %q", tcCase.predicate)
			}
			if tcCase.storage != nil {
				assertStorageReference(t, stored.StorageRef, tcCase.storage)
			}
		})
	}

	assertCounterZero(t, metricsRegistry, "semstreams_graph_ingest_indexing_profile_default_total")
	assertCounterZero(t, metricsRegistry, "semstreams_graph_ingest_mutation_rejections_total")
	assertCounterZero(t, metricsRegistry, "semstreams_graph_ingest_foreign_edge_unclaimed_total")
	assertCounterZero(t, metricsRegistry, "semstreams_graph_ingest_foreign_edge_dropped_total")
	assertCounterZero(t, metricsRegistry, "semstreams_graph_ingest_owner_lease_mismatch_total")
}

func TestIntegration_SyntheticBinaryProofPublishesGovernedMetadata(t *testing.T) {
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

	store, err := filestore.New(t.TempDir(), false)
	if err != nil {
		t.Fatalf("filestore.New() error = %v", err)
	}
	fixturePath := filepath.Join(t.TempDir(), "synthetic-binary-proof.bin")
	fixtureBytes := []byte{0x53, 0x45, 0x4d, 0x2d, 0x42, 0x49, 0x4e, 0x00, 0x01, 0xfe, 0xff}
	if err := os.WriteFile(fixturePath, fixtureBytes, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := binaryproof.BuildSyntheticFixture(
		ctx,
		"acme",
		fixturePath,
		store,
		binaryproof.DefaultStorageInstance,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("BuildSyntheticFixture() error = %v", err)
	}

	reader, err := ownership.NewClaimReader(ctx, tc.Client, nil)
	if err != nil {
		t.Fatalf("NewClaimReader() error = %v", err)
	}
	assertOwnedPredicate(t, ctx, reader, result.Payload.ID, source.MediaStorageRef)
	assertOwnedPredicate(t, ctx, reader, result.Payload.ID, source.MediaByteRange)
	assertOwnedPredicate(t, ctx, reader, result.Payload.ID, source.MediaExtractionFinding)

	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	metricsRegistry := metric.NewMetricsRegistry()
	ingest := startGraphIngest(t, ctx, tc.Client, reg, metricsRegistry)
	t.Cleanup(func() {
		_ = ingest.Stop(5 * time.Second)
	})

	publishSemsourcePayload(t, ctx, tc.Client, result.Payload)
	stored := waitForEntityState(t, ctx, tc.Client, result.Payload.ID, 5*time.Second)
	if stored.MessageType.Key() != semsourcegraph.EntityType.Key() {
		t.Fatalf("stored MessageType = %q, want %q", stored.MessageType.Key(), semsourcegraph.EntityType.Key())
	}
	if got := profileValues(stored); len(got) != 1 || got[0] != semsourcegraph.IndexingProfileTrace {
		t.Fatalf("indexing profile values = %v, want [trace]", got)
	}
	assertStorageReference(t, stored.StorageRef, result.StorageRef)
	for _, predicate := range []string{
		source.MediaStorageRef,
		source.MediaFileHash,
		source.MediaFileSize,
		source.MediaByteRange,
		source.MediaExtractionFinding,
	} {
		if !hasPredicate(stored, predicate) {
			t.Fatalf("stored entity missing predicate %q", predicate)
		}
	}
	assertNoRawBinaryObjects(t, stored, fixtureBytes)

	assertCounterZero(t, metricsRegistry, "semstreams_graph_ingest_indexing_profile_default_total")
	assertCounterZero(t, metricsRegistry, "semstreams_graph_ingest_mutation_rejections_total")
	assertCounterZero(t, metricsRegistry, "semstreams_graph_ingest_owner_lease_mismatch_total")
}

func TestIntegration_GraphQueryPrefixAndSummaryForSemsourceEntities(t *testing.T) {
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
	metricsRegistry := metric.NewMetricsRegistry()

	ingest := startGraphIngest(t, ctx, tc.Client, reg, metricsRegistry)
	t.Cleanup(func() {
		_ = ingest.Stop(5 * time.Second)
	})
	index := startGraphIndex(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() {
		_ = index.Stop(5 * time.Second)
	})
	query := startGraphQuery(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() {
		_ = query.Stop(5 * time.Second)
	})

	entities := []struct {
		id        string
		profile   string
		predicate string
		object    any
	}{
		{
			id:        "acme.semsource.web.docs.doc.aaa111",
			profile:   semsourcegraph.IndexingProfileContent,
			predicate: source.DocContent,
			object:    "first governed graph document",
		},
		{
			id:        "acme.semsource.web.docs.doc.bbb222",
			profile:   semsourcegraph.IndexingProfileContent,
			predicate: source.DocContent,
			object:    "second governed graph document",
		},
		{
			id:        "acme.semsource.config.repo.gomod.app",
			profile:   semsourcegraph.IndexingProfileControl,
			predicate: source.ConfigModulePath,
			object:    "github.com/acme/app",
		},
	}

	for _, entity := range entities {
		publishSemsourceEntity(t, ctx, tc.Client, entity.id, entity.profile, []message.Triple{
			{Subject: entity.id, Predicate: entity.predicate, Object: entity.object},
		})
		waitForEntityState(t, ctx, tc.Client, entity.id, 5*time.Second)
	}

	waitForPredicateIndexed(t, ctx, tc.Client, source.DocContent, 2, 5*time.Second)

	predicateClient, err := graphqueryclient.NewClient(ctx, tc.Client, graphqueryclient.DefaultConfig())
	if err != nil {
		t.Fatalf("create graph query client: %v", err)
	}
	t.Cleanup(func() {
		_ = predicateClient.Close()
	})
	predicateEntities, err := predicateClient.GetEntitiesByPredicate(ctx, source.DocContent)
	if err != nil {
		t.Fatalf("query canonical predicate %q: %v", source.DocContent, err)
	}
	wantPredicateEntities := map[string]bool{
		"acme.semsource.web.docs.doc.aaa111": false,
		"acme.semsource.web.docs.doc.bbb222": false,
	}
	for _, entityID := range predicateEntities {
		if _, ok := wantPredicateEntities[entityID]; ok {
			wantPredicateEntities[entityID] = true
		}
	}
	for entityID, found := range wantPredicateEntities {
		if !found {
			t.Errorf("canonical predicate %q missing known entity %q; got %v", source.DocContent, entityID, predicateEntities)
		}
	}

	firstPage := requestPrefixPage(t, ctx, tc.Client, semgraph.PrefixQueryRequest{
		Prefix: "acme.semsource.web.docs",
		Limit:  1,
	})
	if len(firstPage.Entities) != 1 {
		t.Fatalf("first prefix page entity count = %d, want 1", len(firstPage.Entities))
	}
	if firstPage.NextCursor == "" {
		t.Fatal("first prefix page next_cursor is empty; want pagination cursor")
	}

	secondPage := requestPrefixPage(t, ctx, tc.Client, semgraph.PrefixQueryRequest{
		Prefix: "acme.semsource.web.docs",
		Limit:  1,
		Cursor: firstPage.NextCursor,
	})
	if len(secondPage.Entities) != 1 {
		t.Fatalf("second prefix page entity count = %d, want 1", len(secondPage.Entities))
	}
	if secondPage.NextCursor != "" {
		t.Fatalf("second prefix page next_cursor = %q, want exhausted", secondPage.NextCursor)
	}
	if firstPage.Entities[0].ID == secondPage.Entities[0].ID {
		t.Fatalf("prefix pagination returned duplicate entity %q", firstPage.Entities[0].ID)
	}

	summary := requestGraphSummary(t, ctx, tc.Client)
	if summary.TotalEntities < len(entities) {
		t.Fatalf("summary total_entities = %d, want at least %d", summary.TotalEntities, len(entities))
	}
	if !hasEntityTypeSummary(summary, "web.docs.doc") {
		t.Fatalf("summary missing web.docs.doc type bucket: %#v", summary.EntityTypes)
	}
	if !hasPredicateSummary(summary, source.DocContent, 2) {
		t.Fatalf("summary missing %q predicate count >= 2: %#v", source.DocContent, summary.Predicates)
	}
}

func publishSemsourceEntity(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	entityID string,
	profile string,
	triples []message.Triple,
	storageRef ...*message.StorageReference,
) {
	t.Helper()

	payload := &semsourcegraph.EntityPayload{
		ID:                  entityID,
		TripleData:          triples,
		UpdatedAt:           time.Now().UTC(),
		IndexingProfileHint: profile,
	}
	if len(storageRef) > 0 {
		payload.Storage = storageRef[0]
	}
	publishSemsourcePayload(t, ctx, client, payload)
}

func publishSemsourcePayload(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	payload *semsourcegraph.EntityPayload,
) {
	t.Helper()

	msg := message.NewBaseMessage(semsourcegraph.EntityType, payload, "semsource")
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal entity message: %v", err)
	}
	if err := client.PublishToStream(ctx, "graph.ingest.entity", data); err != nil {
		t.Fatalf("PublishToStream() error = %v", err)
	}
}

func startGraphIngest(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	reg *payloadregistry.Registry,
	metricsRegistry *metric.MetricsRegistry,
) *graphingest.Component {
	t.Helper()

	configJSON, err := json.Marshal(map[string]any{
		"enforce_owner_lease": false,
		"ports": map[string]any{
			"inputs": []map[string]any{
				{
					"name":        "entity_stream",
					"type":        "jetstream",
					"subject":     "graph.ingest.entity",
					"stream_name": "GRAPH",
					"config":      map[string]any{"deliver_policy": "all"},
				},
			},
			"outputs": []map[string]any{
				{"name": "entity_states", "type": "kv-write", "subject": "ENTITY_STATES"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal graph-ingest config: %v", err)
	}

	discovered, err := graphingest.CreateGraphIngest(configJSON, component.Dependencies{
		NATSClient:      client,
		PayloadRegistry: reg,
		MetricsRegistry: metricsRegistry,
	})
	if err != nil {
		t.Fatalf("CreateGraphIngest() error = %v", err)
	}
	ingest := discovered.(*graphingest.Component)
	if err := ingest.Initialize(); err != nil {
		t.Fatalf("graph-ingest Initialize() error = %v", err)
	}
	if err := ingest.Start(ctx); err != nil {
		t.Fatalf("graph-ingest Start() error = %v", err)
	}
	return ingest
}

func startGraphIndex(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	metricsRegistry *metric.MetricsRegistry,
) *graphindex.Component {
	t.Helper()

	configJSON, err := json.Marshal(map[string]any{
		"startup_attempts":    5,
		"startup_interval_ms": 50,
		"ports": map[string]any{
			"inputs": []map[string]any{
				{"name": "entity_watch", "type": "kv-watch", "subject": "ENTITY_STATES"},
			},
			"outputs": []map[string]any{
				{"name": "outgoing_index", "type": "kv-write", "subject": "OUTGOING_INDEX"},
				{"name": "incoming_index", "type": "kv-write", "subject": "INCOMING_INDEX"},
				{"name": "alias_index", "type": "kv-write", "subject": "ALIAS_INDEX"},
				{"name": "predicate_index", "type": "kv-write", "subject": "PREDICATE_INDEX"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal graph-index config: %v", err)
	}

	discovered, err := graphindex.CreateGraphIndex(configJSON, component.Dependencies{
		NATSClient:      client,
		MetricsRegistry: metricsRegistry,
	})
	if err != nil {
		t.Fatalf("CreateGraphIndex() error = %v", err)
	}
	index := discovered.(*graphindex.Component)
	if err := index.Initialize(); err != nil {
		t.Fatalf("graph-index Initialize() error = %v", err)
	}
	if err := index.Start(ctx); err != nil {
		t.Fatalf("graph-index Start() error = %v", err)
	}
	return index
}

func startGraphQuery(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	metricsRegistry *metric.MetricsRegistry,
) *graphquery.Component {
	t.Helper()

	configJSON, err := json.Marshal(map[string]any{
		"query_timeout":    5 * time.Second,
		"startup_attempts": 1,
		"startup_interval": time.Millisecond,
		"recheck_interval": time.Second,
		"ports": map[string]any{
			"inputs": []map[string]any{
				{"name": "query_entity", "type": "nats-request", "subject": "graph.query.entity"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal graph-query config: %v", err)
	}

	discovered, err := graphquery.CreateGraphQuery(configJSON, component.Dependencies{
		NATSClient:      client,
		MetricsRegistry: metricsRegistry,
	})
	if err != nil {
		t.Fatalf("CreateGraphQuery() error = %v", err)
	}
	query := discovered.(*graphquery.Component)
	if err := query.Initialize(); err != nil {
		t.Fatalf("graph-query Initialize() error = %v", err)
	}
	if err := query.Start(ctx); err != nil {
		t.Fatalf("graph-query Start() error = %v", err)
	}
	return query
}

func waitForEntityState(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	entityID string,
	timeout time.Duration,
) *semgraph.EntityState {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		bucket, err := client.GetKeyValueBucket(ctx, semgraph.BucketEntityStates)
		if err != nil {
			lastErr = err
			time.Sleep(25 * time.Millisecond)
			continue
		}
		entry, err := client.NewKVStore(bucket).Get(ctx, entityID)
		if err != nil {
			lastErr = err
			time.Sleep(25 * time.Millisecond)
			continue
		}
		var stored semgraph.EntityState
		if err := json.Unmarshal(entry.Value, &stored); err != nil {
			t.Fatalf("unmarshal stored entity: %v", err)
		}
		return &stored
	}
	t.Fatalf("timed out waiting for %s in ENTITY_STATES; last error: %v", entityID, lastErr)
	return nil
}

func profileValues(entity *semgraph.EntityState) []string {
	var values []string
	for _, triple := range entity.Triples {
		if triple.Predicate != semvocab.EntityIndexingProfile {
			continue
		}
		value, ok := triple.Object.(string)
		if ok {
			values = append(values, value)
		}
	}
	return values
}

func hasPredicate(entity *semgraph.EntityState, predicate string) bool {
	for _, triple := range entity.Triples {
		if triple.Predicate == predicate {
			return true
		}
	}
	return false
}

func assertOwnedPredicate(
	t *testing.T,
	ctx context.Context,
	reader *ownership.ClaimReader,
	entityID string,
	predicate string,
) {
	t.Helper()

	owner, _, ok, err := reader.OwnerOf(ctx, entityID, predicate)
	if err != nil {
		t.Fatalf("OwnerOf(%s, %s) error = %v", entityID, predicate, err)
	}
	if !ok {
		t.Fatalf("OwnerOf(%s, %s) ok = false", entityID, predicate)
	}
	if owner != OwnerID {
		t.Fatalf("OwnerOf(%s, %s) owner = %q, want %q", entityID, predicate, owner, OwnerID)
	}
}

func assertNoRawBinaryObjects(t *testing.T, entity *semgraph.EntityState, raw []byte) {
	t.Helper()

	rawString := string(raw)
	for _, triple := range entity.Triples {
		if got, ok := triple.Object.([]byte); ok && string(got) == rawString {
			t.Fatalf("triple %s contains raw bytes", triple.Predicate)
		}
		if got, ok := triple.Object.(string); ok && got == rawString {
			t.Fatalf("triple %s contains raw bytes as string", triple.Predicate)
		}
	}
}

func assertStorageReference(t *testing.T, got, want *message.StorageReference) {
	t.Helper()

	if got == nil {
		t.Fatal("stored StorageRef is nil")
	}
	if got.StorageInstance != want.StorageInstance {
		t.Fatalf("StorageInstance = %q, want %q", got.StorageInstance, want.StorageInstance)
	}
	if got.Key != want.Key {
		t.Fatalf("StorageRef key = %q, want %q", got.Key, want.Key)
	}
	if got.ContentType != want.ContentType {
		t.Fatalf("StorageRef content_type = %q, want %q", got.ContentType, want.ContentType)
	}
	if got.Size != want.Size {
		t.Fatalf("StorageRef size = %d, want %d", got.Size, want.Size)
	}
}

func waitForPredicateIndexed(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	predicate string,
	minCount int,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := requestPredicateList(ctx, client)
		if err != nil {
			lastErr = err
			time.Sleep(25 * time.Millisecond)
			continue
		}
		for _, summary := range resp.Data.Predicates {
			if summary.Predicate == predicate && summary.EntityCount >= minCount {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for predicate %q count >= %d; last error: %v", predicate, minCount, lastErr)
}

func requestPredicateList(
	ctx context.Context,
	client *natsclient.Client,
) (semgraph.PredicateListQueryResponse, error) {
	// ADR-060: query failures arrive as a typed *errs.ClassifiedError on the err
	// channel; a success reply is the body-only QueryResponse envelope (no Error field).
	body, err := client.RequestClassified(ctx, "graph.index.query.predicateList", []byte("{}"), 5*time.Second)
	if err != nil {
		return semgraph.PredicateListQueryResponse{}, err
	}
	var resp semgraph.PredicateListQueryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return semgraph.PredicateListQueryResponse{}, err
	}
	return resp, nil
}

func requestPrefixPage(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	req semgraph.PrefixQueryRequest,
) semgraph.PrefixQueryResponse {
	t.Helper()

	reqBody, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal prefix request: %v", err)
	}
	body, err := client.RequestClassified(ctx, "graph.query.prefix", reqBody, 5*time.Second)
	if err != nil {
		t.Fatalf("graph.query.prefix request error: %v", err)
	}
	var resp semgraph.PrefixQueryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal prefix response: %v; body=%s", err, body)
	}
	return resp
}

func requestGraphSummary(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
) semgraph.SummaryData {
	t.Helper()

	reqBody, err := json.Marshal(semgraph.SummaryRequest{
		IncludePredicates: true,
		EntitySampleLimit: 10,
		ExamplesPerType:   2,
	})
	if err != nil {
		t.Fatalf("marshal summary request: %v", err)
	}
	body, err := client.RequestClassified(ctx, "graph.query.summary", reqBody, 5*time.Second)
	if err != nil {
		t.Fatalf("graph.query.summary request error: %v", err)
	}
	var resp semgraph.QueryResponse[semgraph.SummaryData]
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal summary response: %v; body=%s", err, body)
	}
	return resp.Data
}

func hasEntityTypeSummary(summary semgraph.SummaryData, entityType string) bool {
	for _, typeSummary := range summary.EntityTypes {
		if typeSummary.Type == entityType && typeSummary.Count > 0 && len(typeSummary.Examples) > 0 {
			return true
		}
	}
	return false
}

func hasPredicateSummary(summary semgraph.SummaryData, predicate string, minCount int) bool {
	for _, predicateSummary := range summary.Predicates {
		if predicateSummary.Predicate == predicate && predicateSummary.EntityCount >= minCount {
			return true
		}
	}
	return false
}

func assertCounterZero(t *testing.T, registry *metric.MetricsRegistry, name string) {
	t.Helper()

	families, err := registry.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, sample := range family.GetMetric() {
			if sample.GetCounter().GetValue() != 0 {
				t.Fatalf("%s = %v, want 0", name, sample.GetCounter().GetValue())
			}
		}
		return
	}
}
