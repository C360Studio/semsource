package astsource

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	semsourceast "github.com/c360studio/semsource/source/ast"
)

func TestIsMinifiedName(t *testing.T) {
	for name, want := range map[string]bool{
		"plotly-latest.min.js":  true,
		"plotly-latest-min.js":  true,
		"styles.min.css":        true,
		"theme-min.css":         true,
		"Plotly-Latest-MIN.JS":  true,  // case-insensitive
		"admin.js":              false, // "min.js" only as a suffix SEGMENT
		"minify.js":             false,
		"terminal.css":          false,
		"main.js":               false,
		"plotly-latest.min.txt": false, // convention is js/css only
	} {
		if got := isMinifiedName(name); got != want {
			t.Errorf("isMinifiedName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsMinifiedContent(t *testing.T) {
	minified := []byte(strings.Repeat("var a=1;", 2048)) // one huge line
	handWritten := []byte(strings.Repeat("func ok() {\n\treturn\n}\n\n", 600))
	// Real code with occasional long lines but ordinary structure: mean stays
	// far under the threshold.
	longButReal := []byte(strings.Repeat("x := 1\n", 500) + strings.Repeat("a", 2000) + "\n" + strings.Repeat("y := 2\n", 500))

	if !isMinifiedContent(minified, int64(len(minified))) {
		t.Error("single-huge-line content must read as minified")
	}
	if isMinifiedContent(handWritten, int64(len(handWritten))) {
		t.Error("hand-written-shaped content must not read as minified")
	}
	if isMinifiedContent(longButReal, int64(len(longButReal))) {
		t.Error("occasional long lines in real code must not read as minified")
	}
	// Small files are exempt however they are shaped — they cannot flood.
	small := []byte(strings.Repeat("a", 2000))
	if isMinifiedContent(small, int64(len(small))) {
		t.Error("files under the probe minimum must be exempt")
	}
}

// TestMinifiedFileResult_FileEntityIdentity is the identity lockstep guard:
// the entity the guard fabricates must be byte-identical (ID and hash) to the
// file entity the routed parser would have produced. If a parser's domain or
// file-entity construction changes, this fails rather than the graph forking.
func TestMinifiedFileResult_FileEntityIdentity(t *testing.T) {
	root := t.TempDir()
	// Minified NAME so the guard fires, but parseable content so the real
	// parser can produce its file entity for comparison.
	path := filepath.Join(root, "widget.min.js")
	if err := os.WriteFile(path, []byte("var a = 1;\nfunction f() { return a; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	parser, err := semsourceast.DefaultRegistry.CreateParser("javascript", "acme", "proj", root)
	if err != nil {
		t.Fatalf("create parser: %v", err)
	}
	parsed, err := parser.ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if parsed.FileEntity == nil {
		t.Fatal("parser produced no file entity")
	}

	c := &Component{logger: slog.Default()}
	pw := &pathWatcher{
		root:         root,
		scopedSystem: "proj",
		config:       WatchPathConfig{Org: "acme", Project: "proj"},
	}
	guarded := c.minifiedFileResult(pw, path, "javascript")
	if guarded == nil {
		t.Fatal("guard must detect a .min.js name")
	}
	if guarded.FileEntity.ID != parsed.FileEntity.ID {
		t.Errorf("guard file-entity ID %q != parser's %q — identity fork", guarded.FileEntity.ID, parsed.FileEntity.ID)
	}
	if guarded.Hash != parsed.Hash {
		t.Errorf("guard hash %q != parser hash %q", guarded.Hash, parsed.Hash)
	}
	if len(guarded.Entities) != 1 {
		t.Errorf("guard result must carry exactly the file entity, got %d entities", len(guarded.Entities))
	}
}

// TestEntityDomainMatchesParser pins the guard's domain mapping to the TS
// parser's per-extension behavior and the identity of every other route.
func TestEntityDomainMatchesParser(t *testing.T) {
	for _, tc := range []struct{ route, ext, want string }{
		{"typescript", ".ts", "typescript"},
		{"typescript", ".js", "javascript"},
		{"javascript", ".js", "javascript"},
		{"javascript", ".mjs", "javascript"},
		{"java", ".java", "java"},
		{"golang", ".go", "golang"},
		{"python", ".py", "python"},
	} {
		if got := entityDomainForRoute(tc.route, tc.ext); got != tc.want {
			t.Errorf("entityDomainForRoute(%q, %q) = %q, want %q", tc.route, tc.ext, got, tc.want)
		}
	}
}

// TestParseFileWithWatcher_MinifiedSkipsSymbols — the wired guard: a minified
// file comes back as one file entity (counted as parsed, counted as
// minified), an ordinary sibling parses exactly as before.
func TestParseFileWithWatcher_MinifiedSkipsSymbols(t *testing.T) {
	root := t.TempDir()
	minPath := filepath.Join(root, "bundle.min.js")
	if err := os.WriteFile(minPath, []byte("var a=1;function f(){return a}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	normalPath := filepath.Join(root, "app.js")
	if err := os.WriteFile(normalPath, []byte("function real() {\n  return 42;\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	parser, err := semsourceast.DefaultRegistry.CreateParser("javascript", "acme", "proj", root)
	if err != nil {
		t.Fatalf("create parser: %v", err)
	}
	pw := &pathWatcher{
		root:         root,
		scopedSystem: "proj",
		config:       WatchPathConfig{Org: "acme", Project: "proj"},
		parsers:      map[string]semsourceast.FileParser{"javascript": parser},
		routes:       map[string]string{".js": "javascript"},
	}
	c := &Component{logger: slog.Default()}

	minRes, err := c.parseFileWithWatcher(context.Background(), pw, minPath)
	if err != nil {
		t.Fatalf("minified parse: %v", err)
	}
	if len(minRes.Entities) != 1 || minRes.Entities[0].Type != semsourceast.TypeFile {
		t.Errorf("minified file must yield exactly its file entity, got %d entities", len(minRes.Entities))
	}

	normalRes, err := c.parseFileWithWatcher(context.Background(), pw, normalPath)
	if err != nil {
		t.Fatalf("normal parse: %v", err)
	}
	var symbols int
	for _, e := range normalRes.Entities {
		if !containerTypes[e.Type] {
			symbols++
		}
	}
	if symbols == 0 {
		t.Error("ordinary sibling must still yield symbol entities")
	}

	if got := c.minifiedFiles.Load(); got != 1 {
		t.Errorf("minifiedFiles = %d, want 1", got)
	}
	if got := c.filesParsed.Load(); got != 2 {
		t.Errorf("filesParsed = %d, want 2 (a guarded file was still handled)", got)
	}
}

func TestEnforceSymbolCap(t *testing.T) {
	mk := func(n int) *semsourceast.ParseResult {
		res := &semsourceast.ParseResult{Path: "big.js"}
		file := &semsourceast.CodeEntity{ID: "f", Type: semsourceast.TypeFile}
		res.FileEntity = file
		res.Entities = append(res.Entities, file)
		for i := 0; i < n; i++ {
			res.Entities = append(res.Entities, &semsourceast.CodeEntity{ID: "s", Type: semsourceast.TypeVar})
		}
		return res
	}

	t.Run("over cap strips symbols, keeps containers", func(t *testing.T) {
		// Capture at Info: the breach line must arrive AT the default level
		// (Warn), not slip below it — the spec's "loudly" is a level contract.
		capture := newLogCapture(slog.LevelInfo)
		c := &Component{logger: slog.New(capture), config: Config{MaxSymbolsPerFile: 10}}
		res := c.enforceSymbolCap(mk(11))
		if len(res.Entities) != 1 || res.Entities[0].Type != semsourceast.TypeFile {
			t.Errorf("breaching file must keep only file-level entities, got %d", len(res.Entities))
		}
		if c.cappedFiles.Load() != 1 || c.cappedSymbols.Load() != 11 {
			t.Errorf("cap counters = (%d files, %d symbols), want (1, 11)", c.cappedFiles.Load(), c.cappedSymbols.Load())
		}
		var warned bool
		capture.mu.Lock()
		for _, r := range capture.records {
			if r.Level >= slog.LevelWarn && strings.Contains(r.Message, "symbol cap breached") {
				warned = true
			}
		}
		capture.mu.Unlock()
		if !warned {
			t.Error("cap breach must be reported at Warn or above")
		}
	})

	t.Run("at cap is untouched", func(t *testing.T) {
		c := &Component{logger: slog.Default(), config: Config{MaxSymbolsPerFile: 10}}
		res := c.enforceSymbolCap(mk(10))
		if len(res.Entities) != 11 {
			t.Errorf("at-cap file must be untouched, got %d entities", len(res.Entities))
		}
		if c.cappedFiles.Load() != 0 {
			t.Error("at-cap file must not count as capped")
		}
	})

	t.Run("zero disables", func(t *testing.T) {
		c := &Component{logger: slog.Default(), config: Config{MaxSymbolsPerFile: 0}}
		res := c.enforceSymbolCap(mk(50))
		if len(res.Entities) != 51 {
			t.Errorf("disabled cap must publish everything, got %d entities", len(res.Entities))
		}
	})
}

func TestConfigValidate_MaxSymbolsPerFile(t *testing.T) {
	base := DefaultConfig()
	base.WatchPaths = []WatchPathConfig{{Path: ".", Org: "acme", Project: "p", Languages: []string{"go"}}}
	if base.MaxSymbolsPerFile != 5000 {
		t.Errorf("default cap = %d, want 5000", base.MaxSymbolsPerFile)
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("default config must validate: %v", err)
	}
	base.MaxSymbolsPerFile = -1
	if err := base.Validate(); err == nil {
		t.Error("negative cap must be rejected")
	}
	base.MaxSymbolsPerFile = 0
	if err := base.Validate(); err != nil {
		t.Errorf("zero (disabled) cap must validate: %v", err)
	}
}

// logCapture collects slog records so tests can assert on emission (or
// silence) at specific levels.
type logCapture struct {
	slog.Handler
	mu      sync.Mutex
	records []slog.Record
}

func newLogCapture(level slog.Level) *logCapture {
	c := &logCapture{}
	c.Handler = slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: level})
	return c
}

func (l *logCapture) Handle(ctx context.Context, r slog.Record) error {
	l.mu.Lock()
	l.records = append(l.records, r)
	l.mu.Unlock()
	return l.Handler.Handle(ctx, r)
}

func (l *logCapture) messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.records))
	for _, r := range l.records {
		out = append(out, r.Message)
	}
	return out
}

// TestGuardSummary — one aggregate line when something was withheld, silence
// on a clean seed.
func TestGuardSummary(t *testing.T) {
	t.Run("nonzero emits exactly one line", func(t *testing.T) {
		capture := newLogCapture(slog.LevelInfo)
		c := &Component{logger: slog.New(capture)}
		c.minifiedFiles.Store(3)
		c.cappedFiles.Store(1)
		c.cappedSymbols.Store(12000)
		c.logGuardSummary()
		msgs := capture.messages()
		if len(msgs) != 1 || msgs[0] != "asset guards withheld symbol extraction" {
			t.Errorf("expected exactly the summary line, got %v", msgs)
		}
	})
	t.Run("clean seed is silent", func(t *testing.T) {
		capture := newLogCapture(slog.LevelInfo)
		c := &Component{logger: slog.New(capture)}
		c.logGuardSummary()
		if msgs := capture.messages(); len(msgs) != 0 {
			t.Errorf("clean seed must emit no guard summary, got %v", msgs)
		}
	})
}

// TestMinifiedSkipLogsAtDebug — per-file guard detail sits below the default
// level: the skip logs at Debug (visible when Debug is on, absent at Info).
func TestMinifiedSkipLogsAtDebug(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "w.min.js")
	if err := os.WriteFile(path, []byte("var a=1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pw := &pathWatcher{root: root, scopedSystem: "p", config: WatchPathConfig{Org: "o", Project: "p"}}

	capture := newLogCapture(slog.LevelDebug)
	c := &Component{logger: slog.New(capture)}
	if c.minifiedFileResult(pw, path, "javascript") == nil {
		t.Fatal("guard must fire")
	}
	found := false
	for _, m := range capture.messages() {
		if strings.Contains(m, "minified asset") {
			found = true
		}
	}
	if !found {
		t.Error("guard skip must log per-file detail at Debug")
	}
}
