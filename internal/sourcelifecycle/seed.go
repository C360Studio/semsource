package sourcelifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/c360studio/semstreams/natsclient"
	"sort"
	"strings"

	"github.com/c360studio/semsource/internal/sourceintent"
)

// SealSeed writes bounded immutable manifest entries before its terminal seal.
// A crash before the seal cannot grant freshness to a partial expected set.
func (j *KVJournal) SealSeed(ctx context.Context, m sourceintent.SeedManifest) error {
	if err := j.currentEpoch(ctx, m.Binding); err != nil {
		return err
	}
	digest, err := sourceintent.ManifestDigest(m.Entries)
	if err != nil {
		return err
	}
	if m.Digest != digest {
		return errors.New("seed manifest digest mismatch")
	}
	prefix, err := j.batchPrefix("manifest", m.Binding, m.BatchID)
	if err != nil {
		return err
	}
	for _, entry := range m.Entries {
		id, err := token(entry.EntityID)
		if err != nil {
			return err
		}
		if err := j.immutable(ctx, prefix+id, entry); err != nil {
			return err
		}
	}
	seal := sourceintent.SeedSeal{BatchID: m.BatchID, Initial: m.Initial, Successful: m.Successful, Count: len(m.Entries), Digest: m.Digest, Failure: m.Failure}
	p, err := j.batchPrefix("seal", m.Binding, m.BatchID)
	if err != nil {
		return err
	}
	if err := j.immutable(ctx, p+"terminal", seal); err != nil {
		return err
	}
	for attempt := 0; attempt < 8; attempt++ {
		r, rev, err := j.Get(ctx, m.Binding.Handle)
		if err != nil {
			return err
		}
		if r.Binding != m.Binding {
			return sourceintent.ErrSuperseded
		}
		if m.Initial {
			r.Seed = &seal
		}
		r.Phase = sourceintent.Pending
		if _, err := j.Update(ctx, r, rev); err == nil {
			return nil
		} else if !errors.Is(err, sourceintent.ErrConflict) {
			return err
		}
	}
	return sourceintent.ErrConflict
}

// LoadSeed requires a terminal seal and exact per-entity agreement; receipt count
// alone can never substitute for a successfully enumerated current-ID manifest.
func (j *KVJournal) LoadSeed(ctx context.Context, b sourceintent.Binding, batch string) (sourceintent.SeedManifest, error) {
	m := sourceintent.SeedManifest{Binding: b, BatchID: batch}
	prefix, err := j.batchPrefix("seal", b, batch)
	if err != nil {
		return m, err
	}
	entry, err := j.kv.Get(ctx, prefix+"terminal")
	if err != nil {
		return m, mapKVError(err)
	}
	var seal sourceintent.SeedSeal
	if err := json.Unmarshal(entry.Value, &seal); err != nil {
		return m, err
	}
	if seal.BatchID != batch {
		return m, errors.New("seed seal batch mismatch")
	}
	m.Initial, m.Successful, m.Digest, m.Failure = seal.Initial, seal.Successful, seal.Digest, seal.Failure
	p, err := j.batchPrefix("manifest", b, batch)
	if err != nil {
		return m, err
	}
	keys, err := j.kv.KeysByPrefix(ctx, p)
	if err != nil {
		return m, err
	}
	for _, key := range keys {
		entry, err := j.kv.Get(ctx, key)
		if err != nil {
			return m, err
		}
		var expected sourceintent.ManifestEntry
		if err := json.Unmarshal(entry.Value, &expected); err != nil {
			return m, err
		}
		id, err := token(expected.EntityID)
		if err != nil || key != p+id {
			return m, errors.New("manifest entry key mismatch")
		}
		m.Entries = append(m.Entries, expected)
	}
	digest, err := sourceintent.ManifestDigest(m.Entries)
	if err != nil {
		return m, err
	}
	if len(m.Entries) != seal.Count || digest != seal.Digest {
		return m, fmt.Errorf("seed manifest incomplete: expected %d entries", seal.Count)
	}
	return m, nil
}

// ListSeeds inventories only fully sealed batches for one exact boot epoch.
// Scanning seals, rather than wake notifications, repairs crashes after sealing.
func (j *KVJournal) ListSeeds(ctx context.Context, b sourceintent.Binding) ([]sourceintent.SeedManifest, error) {
	if err := j.currentEpoch(ctx, b); err != nil {
		return nil, err
	}
	prefix, err := j.epochPrefix("seal", b)
	if err != nil {
		return nil, err
	}
	keys, err := j.kv.KeysByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	out := make([]sourceintent.SeedManifest, 0, len(keys))
	for _, key := range keys {
		suffix := strings.TrimPrefix(key, prefix)
		if !strings.HasSuffix(suffix, ".terminal") {
			return nil, errors.New("invalid seed seal key")
		}
		raw, err := natsclient.DecodeKVOpaqueToken(strings.TrimSuffix(suffix, ".terminal"))
		if err != nil {
			return nil, err
		}
		manifest, err := j.LoadSeed(ctx, b, string(raw))
		if err != nil {
			return nil, err
		}
		out = append(out, manifest)
	}
	return out, nil
}
