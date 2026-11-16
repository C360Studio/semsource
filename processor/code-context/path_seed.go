package codecontext

import (
	"context"
	"path"
	"strings"

	"github.com/c360studio/semengine/pkg/fusion"

	"github.com/c360studio/semsource/entityid"
	handlerdoc "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion/lens/code"
	"github.com/c360studio/semsource/source/fusion/lens/docs"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// pathSeedClient answers the file-path queries both lenses route to prefix mode
// (#276). On the wire, prefix mode takes only an entity-ID prefix: a path with a
// "/" is refused as invalid, and any other path reads as an ID prefix and finds
// nothing. A path is a triple on an entity, not part of its ID, so this
// decorator resolves it here and never sends a path to prefix search:
//
//   - code: the exact name lookup on the file's base name (a file entity's
//     display name), kept to file entities whose code.artifact.path is the path.
//   - docs: the document entity ID each boot source system would give that
//     logical path, kept to documents whose source.doc.file-path is the path.
//
// Both are exact and deterministic, so the prefix provenance stays honest. A
// query that is not a path (the docs lens's entity-ID prefixes) passes through.
type pathSeedClient struct {
	fusion.RetrievalClient
	lens      string
	authority entityid.Authority
	systems   []string
}

func (c pathSeedClient) Resolve(ctx context.Context, q fusion.ResolveQuery) (fusion.Resolution, error) {
	if q.Mode != fusion.ResolveModePrefix {
		return c.RetrievalClient.Resolve(ctx, q)
	}
	if c.lens == "docs" {
		if !docs.IsPath(q.Query) {
			return c.RetrievalClient.Resolve(ctx, q)
		}
		return c.resolveDocument(ctx, cleanPath(q.Query))
	}
	if !code.IsPath(q.Query) {
		return c.RetrievalClient.Resolve(ctx, q)
	}
	return c.resolveCodeFile(ctx, cleanPath(q.Query), q.Limit)
}

// limitSeeds applies the caller's limit after path filtering (zero or less: none).
func limitSeeds(res fusion.Resolution, limit int) fusion.Resolution {
	if limit > 0 && len(res.Seeds) > limit {
		res.Seeds = res.Seeds[:limit]
	}
	return res
}

// cleanPath normalizes a query path to the slash-delimited, root-relative form
// the path triples carry.
func cleanPath(query string) string {
	return strings.TrimPrefix(path.Clean(strings.ReplaceAll(strings.TrimSpace(query), `\`, "/")), "./")
}

func (c pathSeedClient) resolveCodeFile(ctx context.Context, p string, limit int) (fusion.Resolution, error) {
	// Every file with this base name, unlimited: the caller's limit applies after
	// the path filter, or the file asked for could rank past it among same-named
	// files and read as absent. The name index bounds the read itself and refuses
	// with an error past its budget.
	res, err := c.RetrievalClient.Resolve(ctx, fusion.ResolveQuery{
		Query: path.Base(p), Mode: fusion.ResolveModeSymbol,
	})
	if err != nil || len(res.Seeds) == 0 {
		return fusion.Resolution{}, err
	}
	kept, err := c.keep(ctx, fusion.SeedIDs(res.Seeds), func(e *fusion.Entity) bool {
		return e.First(ast.CodeType) == string(ast.TypeFile) && e.First(ast.CodePath) == p
	})
	return limitSeeds(kept, limit), err
}

func (c pathSeedClient) resolveDocument(ctx context.Context, p string) (fusion.Resolution, error) {
	if c.authority.Validate() != nil || len(c.systems) == 0 {
		// No authority or source set (standalone/tests): no document ID can be
		// built, so the honest answer is an empty seed set, the engine's miss path.
		return fusion.Resolution{}, nil
	}
	// shortcut: a bare document name matches only a document at that root-relative
	// path ("QUICKSTART.md" does not find docs/QUICKSTART.md), because a document's
	// display name is its heading, not its file name; upgrade when a consumer needs
	// base-name lookup for documents (an index on source.doc.file-path base names).
	ids := make([]string, 0, len(c.systems))
	for _, system := range c.systems {
		ids = append(ids, handlerdoc.DocumentEntityID(c.authority, system, p))
	}
	return c.keep(ctx, ids, func(e *fusion.Entity) bool {
		return e.First(source.DocFilePath) == p
	})
}

// keep hydrates ids and returns, in order, the seeds whose entity matches. Only
// an ID the graph reports not found is dropped. A candidate that could not be
// read (unhydrated error or unknown) is kept as a seed, because whether it
// matches cannot be known: the engine then fails to hydrate it again and reports
// it under the response's unhydrated seeds (deferring when every seed is), never
// as an ordinary miss. A backend failure stays a failure, so a degraded graph
// never reads as "no such file".
func (c pathSeedClient) keep(ctx context.Context, ids []string, match func(*fusion.Entity) bool) (fusion.Resolution, error) {
	hydration, err := c.RetrievalClient.Entities(ctx, ids)
	if err != nil {
		return fusion.Resolution{}, err
	}
	matched := make(map[string]bool, len(hydration.Entities))
	for _, e := range hydration.Entities {
		if match(e) {
			matched[e.ID] = true
		}
	}
	for _, u := range hydration.Unhydrated {
		if u.Reason != fusion.UnhydratedNotFound {
			matched[u.Handle] = true
		}
	}
	seeds := make([]fusion.Seed, 0, len(matched))
	for _, id := range ids {
		if matched[id] {
			seeds = append(seeds, fusion.Seed{ID: id})
			delete(matched, id)
		}
	}
	return fusion.Resolution{Seeds: seeds}, nil
}
