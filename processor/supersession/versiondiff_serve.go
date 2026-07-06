package supersession

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/c360studio/semsource/graph"
	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/storage"
	"github.com/c360studio/semstreams/storage/objectstore"
)

// versionDiffSubject is the NATS request/reply subject for the version-diff query
// (sits in the graph.query.* read namespace alongside the other query APIs).
const versionDiffSubject = "graph.query.versionDiff"

// diffRequestTimeout caps a single version-diff (one enumeration + bounded
// hydration), so a slow query cannot run unbounded and a caller disconnect
// cancels it.
const diffRequestTimeout = 30 * time.Second

// maxDiffHTTPBody bounds an HTTP request body (diff requests are small JSON).
const maxDiffHTTPBody = 1 << 20

// buildBodyResolver builds the verbatim-body resolver used to hydrate before/after
// bodies. It mirrors the code-context gateway: prefer the shared StoreRegistry
// (ADR-063), else attach an objectstore over the shared CONTENT bucket. A missing
// store is fatal to Start — a diff that advertises bodies but cannot return them
// is misconfigured, not degraded.
func (c *Component) buildBodyResolver(ctx context.Context) (*fusion.BodyResolver, error) {
	if c.storeRegistry != nil {
		return fusion.NewBodyResolver(c.storeRegistry), nil
	}
	store, err := objectstore.NewStoreWithConfig(ctx, c.client, objectstore.Config{
		BucketName:   graph.BodyStoreBucket,
		InstanceName: graph.BodyStoreInstance,
	})
	if err != nil {
		return nil, fmt.Errorf("attach body store %q: %w", graph.BodyStoreInstance, err)
	}
	return fusion.NewBodyResolver(fusion.MapStoreResolver{graph.BodyStoreInstance: storage.Store(store)}), nil
}

// serveDiff decodes a version-diff request, enumerates the project's versioned
// code entities, computes the changeset, hydrates before/after bodies (budget-
// capped), and returns the marshaled response. It always carries an honest
// readiness envelope and, when a version has no entities, a note distinguishing
// that from a genuine "nothing changed".
func (c *Component) serveDiff(ctx context.Context, body []byte) ([]byte, error) {
	var req VersionDiffRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("decode request: %w", err)
		}
	}
	if req.Project == "" || req.From == "" || req.To == "" {
		return nil, fmt.Errorf("project, from, and to are required")
	}

	ctx, cancel := context.WithTimeout(ctx, diffRequestTimeout)
	defer cancel()

	c.mu.RLock()
	q := c.queryClient
	resolver := c.bodyResolver
	c.mu.RUnlock()
	if q == nil {
		return nil, fmt.Errorf("supersession not started")
	}

	ready, note := c.indexReady(ctx)

	entities, truncated, err := q.QueryPrefixAll(ctx, gtypes.PrefixQueryRequest{Prefix: c.config.Prefix}, c.config.maxEntities())
	if err != nil {
		return nil, fmt.Errorf("enumerate entities: %w", err)
	}
	if truncated {
		note = appendNote(note, "enumeration hit max_entities; result may be incomplete")
	}

	cands := make([]candidate, 0, len(entities))
	var fromCount, toCount int
	for i := range entities {
		cand, ok := candidateFromEntity(entities[i])
		if !ok || cand.project != req.Project {
			continue
		}
		if cand.version == req.From {
			fromCount++
		}
		if cand.version == req.To {
			toCount++
		}
		cands = append(cands, cand)
	}

	resp := VersionDiffResponse{Project: req.Project, From: req.From, To: req.To, Ready: ready, Note: note}

	// A wholly-missing version is not a diff — say so, don't emit a giant
	// added/removed list that reads as a real changeset.
	if fromCount == 0 || toCount == 0 {
		resp.Note = appendNote(resp.Note, missingVersionNote(req, fromCount, toCount))
		resp.Changes = []Change{}
		return json.Marshal(resp)
	}

	changes, counts, pairs := diffCandidates(cands, req.Project, req.From, req.To)
	resp.Counts = counts

	if limit := req.maxSymbols(); len(changes) > limit {
		resp.DroppedSymbols = len(changes) - limit
		resp.Truncated = true
		changes = changes[:limit]
	}

	if req.wantBodies() && resolver != nil {
		resp.OmittedBodies = hydrateBodies(ctx, resolver, changes, pairs, req.maxBodyBytes())
	}
	resp.Changes = changes
	return json.Marshal(resp)
}

// hydrateBodies fills from/to verbatim bodies for the listed changes from their
// offloaded-body handles, bounded by a cumulative byte budget. Entries whose body
// is skipped because the budget is exhausted (or that carry no offloaded body) are
// left empty; returns the count skipped for budget reasons.
func hydrateBodies(ctx context.Context, resolver *fusion.BodyResolver, changes []Change, pairs map[string]sidePair, maxBytes int) int {
	var used, omitted int
	for i := range changes {
		pair := pairs[changeID(changes[i])]
		if pair.from != nil {
			if b, skipped := hydrateOne(ctx, resolver, pair.from, &used, maxBytes); skipped {
				omitted++
			} else {
				changes[i].FromBody = b
			}
		}
		if pair.to != nil {
			if b, skipped := hydrateOne(ctx, resolver, pair.to, &used, maxBytes); skipped {
				omitted++
			} else {
				changes[i].ToBody = b
			}
		}
	}
	return omitted
}

// hydrateOne resolves one candidate's verbatim body if it has an offloaded handle
// and fits the remaining byte budget. Returns (body, skipped): skipped is true
// only when a real body was dropped for budget reasons (not when absent).
func hydrateOne(ctx context.Context, resolver *fusion.BodyResolver, cand *candidate, used *int, maxBytes int) (string, bool) {
	if cand.bodyStore == "" || cand.bodyKey == "" {
		return "", false
	}
	if *used >= maxBytes {
		return "", true
	}
	data, err := resolver.ResolveBody(ctx, &message.StorageReference{StorageInstance: cand.bodyStore, Key: cand.bodyKey})
	if err != nil || len(data) == 0 {
		return "", false
	}
	if *used+len(data) > maxBytes {
		return "", true
	}
	*used += len(data)
	return string(data), false
}

// indexReady reports structural index readiness for the honest envelope. It is
// best-effort: an unavailable status yields (false, note) rather than failing the
// whole query.
func (c *Component) indexReady(ctx context.Context) (bool, string) {
	statusCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, err := c.client.Request(statusCtx, "graph.index.query.status", []byte("{}"), 3*time.Second)
	if err != nil {
		return false, "index status unavailable; diff computed over currently-indexed state"
	}
	var status struct {
		Ready bool `json:"ready"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return false, "index status unreadable; diff computed over currently-indexed state"
	}
	if !status.Ready {
		return false, "structural index still building; diff may be incomplete"
	}
	return true, ""
}

// missingVersionNote explains which requested version resolved to no entities.
func missingVersionNote(req VersionDiffRequest, fromCount, toCount int) string {
	switch {
	case fromCount == 0 && toCount == 0:
		return fmt.Sprintf("no indexed entities for project %q at either version %q or %q", req.Project, req.From, req.To)
	case fromCount == 0:
		return fmt.Sprintf("no indexed entities for project %q at version %q (from)", req.Project, req.From)
	default:
		return fmt.Sprintf("no indexed entities for project %q at version %q (to)", req.Project, req.To)
	}
}

// appendNote joins two note fragments, keeping either when the other is empty.
func appendNote(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "; " + b
	}
}

// RegisterHTTPHandlers mounts POST /<prefix>/versionDiff on the ServiceManager's
// shared mux, so a non-NATS consumer can call the version diff over HTTP/JSON.
func (c *Component) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	path := prefix + "versionDiff"
	mux.HandleFunc(path, c.handleHTTPDiff)
	c.logger.Info("registered HTTP handler", "path", path)
}

// handleHTTPDiff serves the version diff over HTTP/JSON. Internal error detail is
// logged, not echoed across the boundary.
func (c *Component) handleHTTPDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c.mu.RLock()
	started := c.running
	c.mu.RUnlock()
	if !started {
		http.Error(w, "component not started", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDiffHTTPBody))
	if err != nil {
		http.Error(w, "request body too large or unreadable", http.StatusBadRequest)
		return
	}
	data, err := c.serveDiff(r.Context(), body)
	if err != nil {
		c.logger.Warn("version-diff request failed", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}
