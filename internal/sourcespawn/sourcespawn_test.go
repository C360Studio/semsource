package sourcespawn

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semstreams/types"
)

// fakeStore captures KV writes and lets us assert on them.
type fakeStore struct {
	puts    map[string]types.ComponentConfig
	deletes []string
	failPut bool
	failDel bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{puts: make(map[string]types.ComponentConfig)}
}

func (f *fakeStore) PutComponentToKV(_ context.Context, name string, cfg types.ComponentConfig) error {
	if f.failPut {
		return errors.New("kv unavailable")
	}
	f.puts[name] = cfg
	return nil
}

func (f *fakeStore) DeleteComponentFromKV(_ context.Context, name string) error {
	if f.failDel {
		return errors.New("kv unavailable")
	}
	f.deletes = append(f.deletes, name)
	return nil
}

// fakeChecker reports whether a name is in its known set.
type fakeChecker struct {
	known map[string]bool
}

func (f fakeChecker) HasComponent(name string) bool { return f.known[name] }

func TestAdd_FlatTypes_WriteOneComponent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		src         config.SourceEntry
		factoryName string
		wantPrefix  string
	}{
		{
			name:        "git",
			src:         config.SourceEntry{Type: "git", URL: "https://github.com/acme/foo"},
			factoryName: "git-source",
			wantPrefix:  "git-source-",
		},
		{
			name:        "ast",
			src:         config.SourceEntry{Type: "ast", Path: "./src", Language: "go"},
			factoryName: "ast-source",
			wantPrefix:  "ast-source-",
		},
		{
			name:        "docs",
			src:         config.SourceEntry{Type: "docs", Paths: []string{"./docs"}},
			factoryName: "doc-source",
			wantPrefix:  "doc-source-",
		},
		{
			name:        "config",
			src:         config.SourceEntry{Type: "config", Paths: []string{"./go.mod"}},
			factoryName: "cfgfile-source",
			wantPrefix:  "cfgfile-source-",
		},
		{
			name:        "url",
			src:         config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}},
			factoryName: "url-source",
			wantPrefix:  "url-source-",
		},
		{
			name:        "image",
			src:         config.SourceEntry{Type: "image", Paths: []string{"./assets"}},
			factoryName: "image-source",
			wantPrefix:  "image-source-",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newFakeStore()
			results, err := Add(context.Background(), tc.src, store, Options{Org: "acme"})
			if err != nil {
				t.Fatalf("Add: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}
			r := results[0]
			if r.FactoryName != tc.factoryName {
				t.Errorf("FactoryName = %q, want %q", r.FactoryName, tc.factoryName)
			}
			if !strings.HasPrefix(r.InstanceName, tc.wantPrefix) {
				t.Errorf("InstanceName = %q, want prefix %q", r.InstanceName, tc.wantPrefix)
			}
			if !r.Created {
				t.Error("Created = false; expected true with no checker")
			}
			if _, ok := store.puts[r.InstanceName]; !ok {
				t.Errorf("expected KV put for %q, got %v", r.InstanceName, store.puts)
			}
		})
	}
}

func TestAdd_RepoSingleBranch_Expands(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{
		Type:   "repo",
		URL:    "https://github.com/acme/foo",
		Branch: "main",
	}
	results, err := Add(context.Background(), src, store, Options{Org: "acme", WorkspaceDir: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Repo expands to git + ast + docs + config = 4 components.
	if len(results) != 4 {
		t.Fatalf("expected 4 results from repo expansion, got %d: %+v", len(results), results)
	}
	wantFactories := map[string]bool{
		"git-source":     false,
		"ast-source":     false,
		"doc-source":     false,
		"cfgfile-source": false,
	}
	for _, r := range results {
		if _, ok := wantFactories[r.FactoryName]; !ok {
			t.Errorf("unexpected factory %q", r.FactoryName)
			continue
		}
		wantFactories[r.FactoryName] = true
	}
	for f, seen := range wantFactories {
		if !seen {
			t.Errorf("expected factory %q in results", f)
		}
	}
	if len(store.puts) != 4 {
		t.Errorf("expected 4 KV puts, got %d", len(store.puts))
	}
}

// TestAdd_RepoBranchSlug_Propagates is a regression test for the branch-watcher
// path: when sourcespawn.Build is called with a "repo" entry carrying a
// BranchSlug, every expanded child must carry that slug so per-branch instance
// names stay distinct in KV.
func TestAdd_RepoBranchSlug_Propagates(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{
		Type:       "repo",
		Path:       "/tmp/worktrees/scenario-auth-flow",
		Branch:     "scenario/auth-flow",
		BranchSlug: "scenario-auth-flow",
	}
	results, err := Add(context.Background(), src, store, Options{Org: "acme", WorkspaceDir: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	for _, r := range results {
		if !strings.HasSuffix(r.InstanceName, "-scenario-auth-flow") {
			t.Errorf("InstanceName = %q for factory %q; expected suffix \"-scenario-auth-flow\"",
				r.InstanceName, r.FactoryName)
		}
	}
}

func TestAdd_RepoMultiBranch_Unsupported(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{
		Type:     "repo",
		URL:      "https://github.com/acme/foo",
		Branches: []string{"*"},
	}
	_, err := Add(context.Background(), src, store, Options{Org: "acme"})
	if CodeOf(err) != CodeUnsupportedType {
		t.Fatalf("CodeOf = %q, want %q (err=%v)", CodeOf(err), CodeUnsupportedType, err)
	}
	if len(store.puts) != 0 {
		t.Errorf("expected no KV writes on unsupported, got %d", len(store.puts))
	}
}

func TestAdd_ValidationFails_NoKVWrite(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{Type: "git"} // missing URL
	_, err := Add(context.Background(), src, store, Options{Org: "acme"})
	if CodeOf(err) != CodeValidationFailed {
		t.Fatalf("CodeOf = %q, want %q", CodeOf(err), CodeValidationFailed)
	}
	if len(store.puts) != 0 {
		t.Errorf("expected no KV writes on validation failure, got %d", len(store.puts))
	}
}

func TestAdd_KVFailure_ReportsKVCode(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.failPut = true
	src := config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}
	_, err := Add(context.Background(), src, store, Options{Org: "acme"})
	if CodeOf(err) != CodeKVWriteFailed {
		t.Fatalf("CodeOf = %q, want %q", CodeOf(err), CodeKVWriteFailed)
	}
}

// flakyStore fails after N successful puts. Used to exercise repo-expansion
// partial-success: writes 1..N-1 land, write N fails, write N+1 never runs.
type flakyStore struct {
	puts    map[string]types.ComponentConfig
	deletes []string
	failAt  int // 1-based; 0 disables
	count   int
}

func newFlakyStore(failAt int) *flakyStore {
	return &flakyStore{puts: make(map[string]types.ComponentConfig), failAt: failAt}
}

func (f *flakyStore) PutComponentToKV(_ context.Context, name string, cfg types.ComponentConfig) error {
	f.count++
	if f.failAt > 0 && f.count == f.failAt {
		return errors.New("kv unavailable on write " + itoa(f.count))
	}
	f.puts[name] = cfg
	return nil
}

func (f *flakyStore) DeleteComponentFromKV(_ context.Context, name string) error {
	f.deletes = append(f.deletes, name)
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// TestAdd_PartialSuccess_ReturnsResultsAndError verifies the contract that
// callers depend on for retry safety: when a repo write fails mid-loop, the
// caller sees both the partial results (committed instances) and the error.
func TestAdd_PartialSuccess_ReturnsResultsAndError(t *testing.T) {
	t.Parallel()
	// Repo expands to 4 components; fail on the 3rd write.
	store := newFlakyStore(3)
	src := config.SourceEntry{
		Type:   "repo",
		URL:    "https://github.com/acme/foo",
		Branch: "main",
	}
	results, err := Add(context.Background(), src, store, Options{Org: "acme", WorkspaceDir: "/tmp/ws"})
	if err == nil {
		t.Fatal("Add returned nil error; want KV failure")
	}
	if CodeOf(err) != CodeKVWriteFailed {
		t.Errorf("CodeOf = %q, want %q", CodeOf(err), CodeKVWriteFailed)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2 (committed before failure)", len(results))
	}
	if len(store.puts) != 2 {
		t.Errorf("len(store.puts) = %d, want 2", len(store.puts))
	}
}

func TestAddWithChecker_DetectsExisting(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}

	// First add: no checker, Created should be true.
	first, err := Add(context.Background(), src, store, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if !first[0].Created {
		t.Fatal("first Created = false; want true")
	}

	// Second add with a checker that knows the instance exists.
	checker := fakeChecker{known: map[string]bool{first[0].InstanceName: true}}
	second, err := AddWithChecker(context.Background(), src, store, checker, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("second Add: %v", err)
	}
	if second[0].Created {
		t.Error("second Created = true; want false (existing instance)")
	}
}

func TestRemove_DeletesByInstanceName(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	if err := Remove(context.Background(), "url-source-example-com", store); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(store.deletes) != 1 || store.deletes[0] != "url-source-example-com" {
		t.Errorf("deletes = %v, want [url-source-example-com]", store.deletes)
	}
}

func TestRemove_EmptyName_Fails(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	err := Remove(context.Background(), "", store)
	if CodeOf(err) != CodeValidationFailed {
		t.Fatalf("CodeOf = %q, want %q", CodeOf(err), CodeValidationFailed)
	}
}

func TestBuild_EquivalentToAdd_NoKVWrite(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}

	// Build is the marshal-only path; verify it produces the same instance
	// names as Add but never touches the store.
	built, err := Build(src, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(built) != 1 {
		t.Fatalf("len(built) = %d, want 1", len(built))
	}

	results, err := Add(context.Background(), src, store, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, ok := built[results[0].InstanceName]; !ok {
		t.Errorf("Build did not produce instance %q (got %v)", results[0].InstanceName, mapKeys(built))
	}
}

// TestAdd_EmptySlugFallback_IsContentStable guards H3: when the natural
// identifier yields an empty SystemSlug (e.g., a path of "."), the fallback
// must be a deterministic function of source content, not of position. Two
// equivalent SourceEntries — added from any context — must land on the same
// KV key.
func TestAdd_EmptySlugFallback_IsContentStable(t *testing.T) {
	t.Parallel()
	// "./" yields an empty SystemSlug; the fallback path is exercised.
	src := config.SourceEntry{Type: "docs", Paths: []string{"./"}}

	storeA := newFakeStore()
	resA, err := Add(context.Background(), src, storeA, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("Add A: %v", err)
	}
	storeB := newFakeStore()
	resB, err := Add(context.Background(), src, storeB, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("Add B: %v", err)
	}
	if resA[0].InstanceName != resB[0].InstanceName {
		t.Errorf("Add of equivalent sources produced different instance names: %q vs %q",
			resA[0].InstanceName, resB[0].InstanceName)
	}

	// Build (loader path) must match Add (programmatic path) for the same input.
	built, err := Build(src, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := built[resA[0].InstanceName]; !ok {
		t.Errorf("Build produced %v; expected %q", mapKeys(built), resA[0].InstanceName)
	}
}

func mapKeys(m map[string]types.ComponentConfig) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
