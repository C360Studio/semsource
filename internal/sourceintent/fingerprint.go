package sourceintent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/message"
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

// ClonePayload freezes publication evidence without converting JSON numbers to
// float64. The producer may reuse its objects after Send returns.
func ClonePayload(payload *graph.EntityPayload) (*graph.EntityPayload, error) {
	if payload == nil {
		return nil, fmt.Errorf("nil publication payload")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("freeze publication: %w", err)
	}
	// A defined local type bypasses EntityPayload.UnmarshalJSON, whose default
	// interface decoding would coerce source numbers through float64.
	type snapshot graph.EntityPayload
	var cloned snapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&cloned); err != nil {
		return nil, fmt.Errorf("decode frozen publication: %w", err)
	}
	return (*graph.EntityPayload)(&cloned), nil
}

// SourceFingerprint identifies exact source-group facts. Lifecycle and derived
// enrichment facts and assertion timestamps cannot prove producer publication.
// Content references are included when represented by owned source predicates.
func SourceFingerprint(entityID string, triples []message.Triple) (string, error) {
	owned := make(map[string]bool)
	for _, predicate := range graph.OwnedPredicates() {
		owned[predicate] = true
	}
	facts := make(map[string]bool)
	for _, triple := range triples {
		if !owned[triple.Predicate] {
			continue
		}
		if triple.Subject != entityID {
			return "", fmt.Errorf("source fact belongs to another entity")
		}
		object, err := canonicalObject(triple.Object)
		if err != nil {
			return "", err
		}
		data, err := json.Marshal([]string{triple.Subject, triple.Predicate, triple.Datatype, string(object)})
		if err != nil {
			return "", err
		}
		facts[string(data)] = true
	}
	ordered := make([]string, 0, len(facts))
	for fact := range facts {
		ordered = append(ordered, fact)
	}
	sort.Strings(ordered)
	data, err := json.Marshal(struct {
		ID    string
		Facts []string
	}{entityID, ordered})
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

func canonicalObject(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode source fact: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	normalized, err := normalizeValue(decoded)
	if err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

// Tagging every value avoids a numeric-tag object colliding with a user object.
func normalizeValue(value any) (any, error) {
	switch v := value.(type) {
	case json.Number:
		n, ok := new(big.Rat).SetString(string(v))
		if !ok {
			return nil, fmt.Errorf("invalid source number %q", v)
		}
		return []any{"number", n.RatString()}, nil
	case string:
		return []any{"string", v}, nil
	case bool:
		return []any{"bool", v}, nil
	case nil:
		return []any{"null"}, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			var err error
			out[i], err = normalizeValue(item)
			if err != nil {
				return nil, err
			}
		}
		return []any{"array", out}, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			var err error
			out[key], err = normalizeValue(item)
			if err != nil {
				return nil, err
			}
		}
		return []any{"object", out}, nil
	default:
		return nil, fmt.Errorf("unsupported source fact type %T", value)
	}
}

// ManifestDigest is stable across enumeration order and rejects contradictory
// evidence for the same current entity ID.
func ManifestDigest(entries []ManifestEntry) (string, error) {
	ordered := append([]ManifestEntry{}, entries...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].EntityID < ordered[j].EntityID })
	for i, entry := range ordered {
		if entry.EntityID == "" || entry.Fingerprint == "" {
			return "", fmt.Errorf("incomplete manifest entry")
		}
		if i > 0 && ordered[i-1].EntityID == entry.EntityID {
			return "", fmt.Errorf("duplicate manifest entity %s", entry.EntityID)
		}
	}
	data, err := json.Marshal(ordered)
	if err != nil {
		return "", err
	}
	return digest(data), nil
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// PublicationFingerprint includes the producer-declared indexing profile and
// content handle in addition to source predicates. These travel outside triples
// on the payload and are lifted into retained graph state by graph-ingest.
func PublicationFingerprint(payload *graph.EntityPayload) (string, error) {
	if payload == nil {
		return "", fmt.Errorf("nil publication payload")
	}
	return completeFingerprint(payload.ID, payload.TripleData, payload.Storage, payload.IndexingProfileHint)
}

// RetainedFingerprint compares the authoritative graph representation of the
// same evidence. Missing or conflicting profiles fail closed.
func RetainedFingerprint(entityID string, triples []message.Triple, storage *message.StorageReference) (string, error) {
	profile := ""
	for _, triple := range triples {
		if triple.Predicate != semvocab.EntityIndexingProfile {
			continue
		}
		value, ok := triple.Object.(string)
		if !ok || triple.Subject != entityID || !semvocab.IsValidIndexingProfile(value) {
			return "", fmt.Errorf("invalid retained indexing profile")
		}
		if profile != "" && profile != value {
			return "", fmt.Errorf("conflicting retained indexing profiles")
		}
		profile = value
	}
	return completeFingerprint(entityID, triples, storage, profile)
}

func completeFingerprint(entityID string, triples []message.Triple, storage *message.StorageReference, profile string) (string, error) {
	if !semvocab.IsValidIndexingProfile(profile) {
		return "", fmt.Errorf("missing or invalid publication profile")
	}
	source, err := SourceFingerprint(entityID, triples)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(struct {
		Source  string
		Storage *message.StorageReference
		Profile string
	}{source, storage, profile})
	if err != nil {
		return "", err
	}
	return digest(data), nil
}
