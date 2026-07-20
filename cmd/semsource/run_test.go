package main

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semstreams/types"
)

// TestBuildSemstreamsConfig_IncludesSupersession guards that the supersession
// component is spawned in the default set. It serves graph.query.versionDiff
// (behind the code_changes MCP tool); if it drops out of the component map the
// query is unserved and code_changes times out — with no compile error to catch
// it (the factory is registered either way).
func TestBuildSemstreamsConfig_IncludesSupersession(t *testing.T) {
	ssCfg, err := buildSemstreamsConfig(&config.Config{Namespace: "acme"}, "acme")
	if err != nil {
		t.Fatalf("buildSemstreamsConfig() error = %v", err)
	}
	comp, ok := ssCfg.Components["supersession"]
	if !ok {
		t.Fatal("supersession missing from the default component set — graph.query.versionDiff would be unserved (code_changes times out)")
	}
	if comp.Type != types.ComponentTypeProcessor {
		t.Errorf("supersession type = %q, want processor", comp.Type)
	}
	if !comp.Enabled {
		t.Error("supersession component should be enabled")
	}
}

// TestBuildSemstreamsConfig_RequiredComponentSet guards the COMPLETE default
// component spawn map (ci-proof-chain D3), not just supersession
// (TestBuildSemstreamsConfig_IncludesSupersession above): mcp-gateway,
// code-context, doc-context, source-manifest, the graph subsystem, and
// websocket-output are every component the advertised product surface (MCP
// tools, GraphQL, WebSocket stream) depends on. With zero configured sources,
// this is the exact set buildSemstreamsConfig always produces — a dropped
// registration in any contributing builder function fails this test, which is
// the beta.1 bug class (supersession missing from the default set).
func TestBuildSemstreamsConfig_RequiredComponentSet(t *testing.T) {
	ssCfg, err := buildSemstreamsConfig(&config.Config{Namespace: "acme"}, "acme")
	if err != nil {
		t.Fatalf("buildSemstreamsConfig() error = %v", err)
	}

	required := []string{
		"source-manifest",
		"graph-ingest",
		"graph-index",
		"graph-embedding",
		"graph-query",
		"graph-gateway",
		"objectstore",
		"websocket-output",
		"code-context",
		"doc-context",
		"mcp-gateway",
		"supersession",
	}

	got := make([]string, 0, len(ssCfg.Components))
	for name := range ssCfg.Components {
		got = append(got, name)
	}
	sort.Strings(got)

	var missing []string
	for _, name := range required {
		comp, ok := ssCfg.Components[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		if !comp.Enabled {
			t.Errorf("component %q is registered but not enabled", name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("default component set is missing required components %v; got %v", missing, got)
	}
}

func TestGraphSubsystemComponents_ObserveOnlyOwnerLease(t *testing.T) {
	components, err := graphSubsystemComponents(&config.Config{})
	if err != nil {
		t.Fatalf("graphSubsystemComponents() error = %v", err)
	}

	graphIngest, ok := components["graph-ingest"]
	if !ok {
		t.Fatal("graph-ingest component not configured")
	}

	var raw map[string]any
	if err := json.Unmarshal(graphIngest.Config, &raw); err != nil {
		t.Fatalf("unmarshal graph-ingest config: %v", err)
	}

	got, ok := raw["enforce_owner_lease"].(bool)
	if !ok {
		t.Fatalf("enforce_owner_lease missing or not bool: %#v", raw["enforce_owner_lease"])
	}
	if got {
		t.Fatal("enforce_owner_lease should remain false for the first governed migration")
	}
}

// TestGraphSubsystemComponents_ObjectStoreConfigured guards the ADR-0006 standup
// of the framework NATS objectstore (storage.objectstore.api), which backs
// location-independent verbatim content. The producer Put + Lens.Hydrate wiring
// lands later, but the store must be present in the subsystem now.
func TestGraphSubsystemComponents_ObjectStoreConfigured(t *testing.T) {
	components, err := graphSubsystemComponents(&config.Config{})
	if err != nil {
		t.Fatalf("graphSubsystemComponents() error = %v", err)
	}

	objStore, ok := components["objectstore"]
	if !ok {
		t.Fatal("objectstore component not configured")
	}
	if objStore.Type != "storage" {
		t.Errorf("objectstore type = %q, want %q", objStore.Type, "storage")
	}

	var raw map[string]any
	if err := json.Unmarshal(objStore.Config, &raw); err != nil {
		t.Fatalf("unmarshal objectstore config: %v", err)
	}
	if raw["bucket_name"] != "CONTENT" {
		t.Errorf("objectstore bucket_name = %v, want CONTENT", raw["bucket_name"])
	}
}

func TestGraphSubsystemComponents_GraphQueryPortsCoverSemStreamsBeta114(t *testing.T) {
	components, err := graphSubsystemComponents(&config.Config{})
	if err != nil {
		t.Fatalf("graphSubsystemComponents() error = %v", err)
	}

	graphQuery, ok := components["graph-query"]
	if !ok {
		t.Fatal("graph-query component not configured")
	}

	var raw map[string]any
	if err := json.Unmarshal(graphQuery.Config, &raw); err != nil {
		t.Fatalf("unmarshal graph-query config: %v", err)
	}

	inputs := portDefinitions(t, raw, "inputs")
	want := map[string]string{
		"query_entity":          "graph.query.entity",
		"query_entity_by_alias": "graph.query.entityByAlias",
		"query_batch":           "graph.query.batch",
		"query_relationships":   "graph.query.relationships",
		"query_path_search":     "graph.query.pathSearch",
		"query_hierarchy_stats": "graph.query.hierarchyStats",
		"query_prefix":          "graph.query.prefix",
		"query_spatial":         "graph.query.spatial",
		"query_temporal":        "graph.query.temporal",
		"query_semantic":        "graph.query.semantic",
		"query_similar":         "graph.query.similar",
		"local_search":          "graph.query.localSearch",
		"global_search":         "graph.query.globalSearch",
		"query_summary":         "graph.query.summary",
		"query_search_graph":    "graph.query.searchGraph",
	}
	assertPortSubjects(t, inputs, want)
}

// TestGraphSubsystemComponents_GraphGatewayHTTPPortSet guards the fix for the
// gateway failing registration with "port 0": the registry parses the gateway's
// network port from the http input port's SUBJECT (parsePortFromSubject), so the
// subject must encode host:port — a path like "/graphql" parses to 0 and
// registration fails. The GraphQL route itself is GraphQLPath, not this subject.
func TestGraphSubsystemComponents_GraphGatewayHTTPPortSet(t *testing.T) {
	components, err := graphSubsystemComponents(&config.Config{})
	if err != nil {
		t.Fatalf("graphSubsystemComponents() error = %v", err)
	}
	gw, ok := components["graph-gateway"]
	if !ok {
		t.Fatal("graph-gateway not configured")
	}
	var raw map[string]any
	if err := json.Unmarshal(gw.Config, &raw); err != nil {
		t.Fatalf("unmarshal graph-gateway config: %v", err)
	}
	ports, _ := raw["ports"].(map[string]any)
	inputs, _ := ports["inputs"].([]any)
	if len(inputs) == 0 {
		t.Fatal("graph-gateway has no input ports")
	}
	httpPort, _ := inputs[0].(map[string]any)
	subject, _ := httpPort["subject"].(string)
	// The default bind is 0.0.0.0:8082, whose trailing :8082 parses to a valid
	// port. A path-style subject ("/graphql") would parse to 0 and be rejected.
	if subject != "0.0.0.0:8082" {
		t.Fatalf("graph-gateway http input port subject = %q, want a host:port the registry can parse to a valid port (e.g. \"0.0.0.0:8082\")", subject)
	}
}

func TestGraphSubsystemComponents_GraphGatewayAdvertisesQueriesAndMutations(t *testing.T) {
	components, err := graphSubsystemComponents(&config.Config{})
	if err != nil {
		t.Fatalf("graphSubsystemComponents() error = %v", err)
	}

	graphGateway, ok := components["graph-gateway"]
	if !ok {
		t.Fatal("graph-gateway component not configured")
	}

	var raw map[string]any
	if err := json.Unmarshal(graphGateway.Config, &raw); err != nil {
		t.Fatalf("unmarshal graph-gateway config: %v", err)
	}

	outputs := portDefinitions(t, raw, "outputs")
	want := map[string]string{
		"queries":   "graph.query.*",
		"mutations": "graph.mutation.*",
	}
	assertPortSubjects(t, outputs, want)
}

// TestGraphStreamConfig_SubjectsExplicit_NoRPCOverlap pins the GRAPH stream's
// subject list (ci-proof-chain D3) — the sole protection against the
// documented PubAck silent-empty-results footgun: a stream subject filter that
// overlaps a request/reply subject would capture RPC traffic into the stream,
// racing (and usually beating) the real reply. Broadening the list — adding a
// subject, or worse, a wildcard — must be a deliberate, reviewed act that
// fails this test until updated, never a silent drift.
func TestGraphStreamConfig_SubjectsExplicit_NoRPCOverlap(t *testing.T) {
	streams := graphStreamConfig(&config.Config{})
	stream, ok := streams["GRAPH"]
	if !ok {
		t.Fatal("GRAPH stream not configured")
	}

	wantSubjects := []string{
		"graph.ingest.entity",
		"graph.ingest.batch",
		"graph.ingest.manifest",
		"graph.ingest.status",
		"graph.ingest.predicates",
	}
	if !reflect.DeepEqual(stream.Subjects, wantSubjects) {
		t.Fatalf("GRAPH stream Subjects = %v, want %v", stream.Subjects, wantSubjects)
	}

	for _, s := range stream.Subjects {
		if strings.ContainsAny(s, "*>") {
			t.Errorf("GRAPH stream subject %q contains a wildcard; the stream subject list must stay an explicit enumeration", s)
		}
	}

	// Every request/reply subject semsource's own components register — a
	// broadened stream filter must never swallow any of these. Grouped by
	// owner for maintainability; namespace-parameterized subjects (source-
	// manifest's add/remove) use a representative concrete namespace.
	rpcReplySubjects := []string{
		// graph-query input ports (graphQueryInputPorts).
		"graph.query.entity", "graph.query.entityByAlias", "graph.query.batch",
		"graph.query.relationships", "graph.query.pathSearch", "graph.query.hierarchyStats",
		"graph.query.prefix", "graph.query.spatial", "graph.query.temporal",
		"graph.query.semantic", "graph.query.similar", "graph.query.localSearch",
		"graph.query.globalSearch", "graph.query.summary", "graph.query.searchGraph",
		// graph-ingest's own query + mutation subjects (framework-registered
		// unconditionally alongside the GRAPH stream input).
		"graph.ingest.query.entity", "graph.ingest.query.batch",
		"graph.ingest.query.prefix", "graph.ingest.query.suffix",
		"graph.mutation.triple.add", "graph.mutation.triple.add_batch", "graph.mutation.triple.remove",
		"graph.mutation.entity.create", "graph.mutation.entity.create_with_triples",
		"graph.mutation.entity.update", "graph.mutation.entity.update_with_triples",
		"graph.mutation.entity.delete",
		// supersession.
		"graph.supersession.run", "graph.query.versionDiff", "graph.lifecycle.run",
		// source-manifest.
		"graph.query.sources", "graph.query.status", "graph.query.predicates",
		"graph.ingest.add.acme", "graph.ingest.remove.acme",
	}
	for _, subj := range stream.Subjects {
		for _, rpc := range rpcReplySubjects {
			if subjectOverlaps(subj, rpc) {
				t.Errorf("GRAPH stream subject %q overlaps request/reply subject %q — this is the PubAck silent-empty-results footgun", subj, rpc)
			}
		}
	}
}

// subjectOverlaps reports whether two NATS subjects could both match the same
// concrete message subject, per NATS wildcard semantics: "*" matches exactly
// one token, ">" matches one-or-more trailing tokens (only meaningful as the
// final token, which is all this pin needs to check).
func subjectOverlaps(a, b string) bool {
	at := strings.Split(a, ".")
	bt := strings.Split(b, ".")
	i := 0
	for i < len(at) && i < len(bt) {
		if at[i] == ">" || bt[i] == ">" {
			return true
		}
		if at[i] != "*" && bt[i] != "*" && at[i] != bt[i] {
			return false
		}
		i++
	}
	return len(at) == len(bt)
}

func portDefinitions(t *testing.T, raw map[string]any, direction string) map[string]string {
	t.Helper()

	ports, ok := raw["ports"].(map[string]any)
	if !ok {
		t.Fatalf("ports missing or malformed: %#v", raw["ports"])
	}

	defs, ok := ports[direction].([]any)
	if !ok {
		t.Fatalf("%s ports missing or malformed: %#v", direction, ports[direction])
	}

	result := make(map[string]string, len(defs))
	for _, def := range defs {
		port, ok := def.(map[string]any)
		if !ok {
			t.Fatalf("port definition malformed: %#v", def)
		}
		name, nameOK := port["name"].(string)
		subject, subjectOK := port["subject"].(string)
		if !nameOK || !subjectOK {
			t.Fatalf("port name/subject malformed: %#v", port)
		}
		if _, exists := result[name]; exists {
			t.Fatalf("duplicate port name %q", name)
		}
		result[name] = subject
	}
	return result
}

func assertPortSubjects(t *testing.T, got, want map[string]string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("port count = %d, want %d; got %#v", len(got), len(want), got)
	}
	for name, wantSubject := range want {
		gotSubject, ok := got[name]
		if !ok {
			t.Fatalf("missing port %q; got %#v", name, got)
		}
		if gotSubject != wantSubject {
			t.Fatalf("port %q subject = %q, want %q", name, gotSubject, wantSubject)
		}
	}
}
