package astsource

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semsource/internal/entitypub"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// recordingClient captures every message the publisher delivers.
type recordingClient struct {
	mu   sync.Mutex
	msgs [][]byte
}

func (r *recordingClient) PublishToStream(_ context.Context, _ string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, append([]byte(nil), data...))
	return nil
}

func (r *recordingClient) PublishToStreamWithMsgID(ctx context.Context, subject string, data []byte, _ string) error {
	return r.PublishToStream(ctx, subject, data)
}

func (r *recordingClient) containing(needle string) [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out [][]byte
	for _, m := range r.msgs {
		if bytes.Contains(m, []byte(needle)) {
			out = append(out, m)
		}
	}
	return out
}

// TestPublishParseResult_DuplicateIDPublishesFirstDeclarationOnce pins the
// runtime half of the #329 guard: two declarations under one ID must reach
// graph-ingest as ONE entity carrying the FIRST declaration's body, never two
// writes it would merge into a hybrid, and never a body picked by whichever
// offload goroutine finished last.
func TestPublishParseResult_DuplicateIDPublishesFirstDeclarationOnce(t *testing.T) {
	root := t.TempDir()
	src := "describe(() => {\n  const fetcher = () => 1;\n});\ndescribe(() => {\n  const fetcher = () => 22222;\n});\n"
	if err := os.WriteFile(filepath.Join(root, "a.test.ts"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	const dupID = "o.p.s.typescript.function.a-test-ts-fetcher"
	result := &semsourceast.ParseResult{
		Path: "a.test.ts",
		Entities: []*semsourceast.CodeEntity{
			{ID: dupID, Name: "fetcher", Type: semsourceast.TypeFunction, StartLine: 2, EndLine: 2},
			{ID: dupID, Name: "fetcher", Type: semsourceast.TypeFunction, StartLine: 5, EndLine: 5},
		},
	}

	client := &recordingClient{}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	pub, err := entitypub.New(client, logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pub.Start(ctx)
	defer func() { _ = pub.Stop(context.Background()) }()

	c := &Component{
		logger:    logger,
		bodyStore: newFakeStore(),
		publisher: pub,
		distinct:  entitypub.NewDistinctTracker(),
	}
	pw := &pathWatcher{root: root, owner: "ast-source"}

	if err := c.publishParseResult(ctx, result, pw); err != nil {
		t.Fatalf("publishParseResult: %v", err)
	}
	if err := pub.WaitSettled(ctx); err != nil {
		t.Fatalf("publisher did not settle: %v", err)
	}

	got := client.containing(dupID)
	if len(got) != 1 {
		t.Fatalf("published %d messages for %s, want exactly 1", len(got), dupID)
	}
	firstKey := bodyKeyPrefix + hashBody("  const fetcher = () => 1;")
	secondKey := bodyKeyPrefix + hashBody("  const fetcher = () => 22222;")
	if !bytes.Contains(got[0], []byte(firstKey)) {
		t.Errorf("published entity does not carry the first declaration's body key %s", firstKey)
	}
	if bytes.Contains(got[0], []byte(secondKey)) {
		t.Errorf("published entity carries the dropped declaration's body key %s", secondKey)
	}
	if n := c.idCollisions.Load(); n != 1 {
		t.Errorf("id_collisions = %d, want 1", n)
	}
	if r := c.buildStatusReport("watching"); r.IDCollisions != 1 {
		t.Errorf("status report id_collisions = %d, want 1", r.IDCollisions)
	}
}

// TestDropDuplicateIDs_AcrossResultsKeepsFirstInWalkOrder covers the initial
// index's cross-file pass: path sanitizing can fold two files onto one ID
// segment, and the later file in walk order loses only the colliding entity.
func TestDropDuplicateIDs_AcrossResultsKeepsFirstInWalkOrder(t *testing.T) {
	first := &semsourceast.ParseResult{Path: "a/b.ts", Entities: []*semsourceast.CodeEntity{
		{ID: "x.a-b-ts-foo"}, {ID: "x.a-b-ts-bar"},
	}}
	second := &semsourceast.ParseResult{Path: "a-b.ts", Entities: []*semsourceast.CodeEntity{
		{ID: "x.a-b-ts-foo"}, {ID: "x.a-b-ts-baz"},
	}}
	c := &Component{logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))}

	c.dropDuplicateIDs([]*semsourceast.ParseResult{first, second})

	if len(first.Entities) != 2 {
		t.Errorf("first result lost entities: %d left, want 2", len(first.Entities))
	}
	if len(second.Entities) != 1 || second.Entities[0].ID != "x.a-b-ts-baz" {
		t.Errorf("second result = %+v, want only x.a-b-ts-baz", second.Entities)
	}
	if n := c.idCollisions.Load(); n != 1 {
		t.Errorf("id_collisions = %d, want 1", n)
	}
}
