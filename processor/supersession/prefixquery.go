package supersession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semstreams/pkg/errs"
	semtypes "github.com/c360studio/semstreams/pkg/types"

	gtypes "github.com/c360studio/semstreams/graph"
)

// prefixQuerySubject is the canonical graph.query prefix operation this
// adapter speaks. Supersession is its only caller in SemSource.
const prefixQuerySubject = "graph.query.prefix"

// prefixPageTimeout bounds one page request. Pages are server-bounded (the
// responder sets next_cursor when a page hits the payload ceiling), so a
// single page is a small, predictable unit of work.
const prefixPageTimeout = 30 * time.Second

// prefixQuerier is supersession's local, operation-specific adapter over
// graph.query.prefix — the replacement for the deleted aggregate
// graph/query.Client, per the beta.160 query-contract closure. It follows the
// opaque-cursor contract verbatim: the cursor is never parsed or constructed,
// only passed back unchanged.
type prefixQuerier struct {
	client interface {
		RequestClassified(context.Context, string, []byte, time.Duration) ([]byte, error)
	}
}

// isResponseTooLarge reports whether err carries the responder's
// response_too_large classification: a result-size failure — the encoded
// result cannot fit the connected server's max payload — never an
// availability timeout, and never retryable as one.
func isResponseTooLarge(err error) bool {
	var ce *errs.ClassifiedError
	return errors.As(err, &ce) && ce.Code == "response_too_large"
}

// queryPrefixAll pages graph.query.prefix to completion, up to maxEntities.
// The bool result reports truncation by maxEntities, mirroring the deleted
// client's contract. Steady-state query: one transport attempt per page via
// RequestClassified, no retry loop.
func (p prefixQuerier) queryPrefixAll(ctx context.Context, prefix string, maxEntities int) ([]gtypes.EntityState, bool, error) {
	var out []gtypes.EntityState
	cursor := ""
	seen := map[string]bool{"": true}
	ids := make(map[string]bool)
	for {
		req := gtypes.PrefixQueryRequest{Prefix: strings.TrimSuffix(prefix, "."), Cursor: cursor}
		data, err := json.Marshal(req)
		if err != nil {
			return out, false, fmt.Errorf("encode prefix request: %w", err)
		}
		raw, err := p.client.RequestClassified(ctx, prefixQuerySubject, data, prefixPageTimeout)
		if err != nil {
			if isResponseTooLarge(err) {
				return out, false, fmt.Errorf(
					"prefix query %q: page exceeds the server payload ceiling (result-size failure, not availability): %w",
					prefix, err)
			}
			return out, false, fmt.Errorf("prefix query %q: %w", prefix, err)
		}
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return out, false, fmt.Errorf("decode prefix envelope: %w", err)
		}
		if _, ok := envelope["entities"]; !ok {
			return out, false, fmt.Errorf("prefix response lacks entities field")
		}
		var resp gtypes.PrefixQueryResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return out, false, fmt.Errorf("decode prefix page: %w", err)
		}
		for _, entity := range resp.Entities {
			if err := semtypes.ValidateEntityID(entity.ID); err != nil {
				return out, false, fmt.Errorf("invalid enumerated entity: %w", err)
			}
			boundary := strings.TrimSuffix(prefix, ".")
			if boundary != "" && entity.ID != boundary && !strings.HasPrefix(entity.ID, boundary+".") {
				return out, false, fmt.Errorf("prefix page returned out-of-scope entity %q", entity.ID)
			}
			if ids[entity.ID] {
				return out, false, fmt.Errorf("prefix enumeration repeated entity %q", entity.ID)
			}
			ids[entity.ID] = true
			out = append(out, entity)
		}

		if maxEntities > 0 && len(out) >= maxEntities {
			truncated := len(out) > maxEntities || resp.NextCursor != ""
			return out[:maxEntities], truncated, nil
		}
		if resp.NextCursor == "" {
			return out, false, nil
		}
		if seen[resp.NextCursor] {
			return out, false, fmt.Errorf("prefix query returned a cyclic cursor")
		}
		seen[resp.NextCursor] = true
		cursor = resp.NextCursor
	}
}
