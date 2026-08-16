package astsource

// Asset ingestion guards (openspec: asset-ingestion-guards, semsource#175).
// Which files the AST source deliberately does NOT extract symbols from:
// minified assets detected by name or content shape are indexed as their file
// entity only — without ever running tree-sitter — and a per-file symbol cap
// backstops whatever slips through. Measured into existence: one vendored
// plotly-latest-min.js family parsed into ~34.9k single-character entities,
// 45% of an entire corpus's publishes and the whole historical publish
// plateau. A junk entity is worse than an unindexed vendored asset.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	semsourceast "github.com/c360studio/semsource/source/ast"
)

const (
	// minifiedProbeLimit bounds the content probe: 64KiB of a minified bundle
	// is decisive, and the common (non-minified) case pays one page-cache read
	// the parser was about to do anyway.
	minifiedProbeLimit = 64 * 1024

	// minifiedProbeMinSize exempts small files from the probe entirely — a
	// minified file this small cannot flood anything, and short files make
	// bytes-per-line noisy.
	minifiedProbeMinSize = 4 * 1024

	// minifiedBytesPerLine is the content-shape threshold. Hand-written and
	// generated-but-real code measures well under 200 bytes/line on the
	// corpora we score; minified plotly measures in the tens of thousands.
	// Set at ~4x the hand-written worst case because a false "minified"
	// verdict silently unindexes real code — the worse failure.
	minifiedBytesPerLine = 512
)

// minifiedNameSuffixes are the conventional minified-asset name patterns.
// The .css entries are inert until a CSS parser exists; they cost nothing
// and pin the convention.
var minifiedNameSuffixes = []string{".min.js", "-min.js", ".min.css", "-min.css"}

// isMinifiedName reports whether the base name carries a conventional
// minified suffix, case-insensitively.
func isMinifiedName(base string) bool {
	b := strings.ToLower(base)
	for _, suffix := range minifiedNameSuffixes {
		if strings.HasSuffix(b, suffix) {
			return true
		}
	}
	return false
}

// isMinifiedContent applies the bytes-per-line shape rule to a probe sample.
// fileSize is the FULL file size, which gates the probe; the sample may be
// truncated at minifiedProbeLimit.
func isMinifiedContent(sample []byte, fileSize int64) bool {
	if fileSize < minifiedProbeMinSize || len(sample) == 0 {
		return false
	}
	lines := bytes.Count(sample, []byte("\n")) + 1
	return len(sample)/lines > minifiedBytesPerLine
}

// entityDomainForRoute maps a routing-table language to the {domain} segment
// the routed parser would stamp on this file's entities. The TS parser is the
// one parser whose domain varies per extension (detectLanguage in
// source/ast/ts): TypeScript extensions get "typescript", everything else it
// reads gets "javascript". Every other parser's domain equals its route
// language. Kept in lockstep by TestEntityDomainMatchesParser.
func entityDomainForRoute(routeLang, ext string) string {
	if routeLang != "typescript" && routeLang != "javascript" {
		return routeLang
	}
	switch ext {
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	default:
		return "javascript"
	}
}

// minifiedFileResult detects a minified asset and, when detected, returns the
// file-entity-only ParseResult the graph gets instead of a parse. Returns nil
// for ordinary files AND on any I/O trouble — the normal parse path is the
// error-surfacing authority, and a guard must degrade to "no opinion", never
// to "silently unindexed".
//
// The file entity is constructed exactly as the routed parser would have
// built it (same NewCodeEntity inputs, same content hash), so detection can
// never fork an entity identity.
func (c *Component) minifiedFileResult(pw *pathWatcher, filePath, routeLang string) *semsourceast.ParseResult {
	base := filepath.Base(filePath)
	byName := isMinifiedName(base)

	if !byName {
		info, err := os.Stat(filePath)
		if err != nil || info.Size() < minifiedProbeMinSize {
			return nil
		}
		f, err := os.Open(filePath)
		if err != nil {
			return nil
		}
		sample := make([]byte, minifiedProbeLimit)
		n, _ := io.ReadFull(f, sample)
		_ = f.Close()
		if !isMinifiedContent(sample[:n], info.Size()) {
			return nil
		}
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	relPath, err := filepath.Rel(pw.root, filePath)
	if err != nil {
		return nil
	}

	how := "shape"
	if byName {
		how = "name"
	}
	c.logger.Debug("minified asset — indexing file entity only, skipping symbol extraction",
		"path", relPath, "detected_by", how)

	hash := semsourceast.ComputeHash(content)
	ext := strings.ToLower(filepath.Ext(filePath))
	fileEntity := semsourceast.NewCodeEntity(
		pw.config.Org,
		entityDomainForRoute(routeLang, ext),
		pw.scopedSystem,
		semsourceast.TypeFile,
		base,
		relPath,
	)
	fileEntity.Hash = hash
	fileEntity.StartLine = 1
	fileEntity.EndLine = bytes.Count(content, []byte("\n")) + 1

	return &semsourceast.ParseResult{
		FileEntity: fileEntity,
		Entities:   []*semsourceast.CodeEntity{fileEntity},
		Path:       relPath,
		Hash:       hash,
	}
}

// enforceSymbolCap strips a breaching file's symbol entities, keeping the
// file-level (container) ones, and says so ONCE at the default level. The cap
// is the loud backstop behind detection: a file that parses into thousands of
// symbols is never signal, whatever its name and shape claimed.
func (c *Component) enforceSymbolCap(result *semsourceast.ParseResult) *semsourceast.ParseResult {
	limit := c.config.MaxSymbolsPerFile
	if limit <= 0 || result == nil {
		return result
	}
	symbols := 0
	for _, e := range result.Entities {
		if e != nil && !containerTypes[e.Type] {
			symbols++
		}
	}
	if symbols <= limit {
		return result
	}

	kept := make([]*semsourceast.CodeEntity, 0, len(result.Entities)-symbols)
	for _, e := range result.Entities {
		if e != nil && containerTypes[e.Type] {
			kept = append(kept, e)
		}
	}
	c.logger.Warn("symbol cap breached — publishing file-level entities only",
		"path", result.Path,
		"symbols", symbols,
		"cap", limit)
	c.cappedFiles.Add(1)
	c.cappedSymbols.Add(int64(symbols))
	result.Entities = kept
	return result
}

// logGuardSummary emits the one aggregate guard line at seed end, only when
// something was withheld (ADR-0011: control volume by aggregating, never by
// lowering severity). Per-file detail lives at Debug on the guard paths. A
// minified skip's symbol count is unknowable by design — the file is never
// parsed — so symbols are reported for cap breaches only.
func (c *Component) logGuardSummary() {
	minified, capped := c.minifiedFiles.Load(), c.cappedFiles.Load()
	if minified+capped == 0 {
		return
	}
	c.logger.Info("asset guards withheld symbol extraction",
		"minified_files", minified,
		"capped_files", capped,
		"capped_symbols", c.cappedSymbols.Load())
}
