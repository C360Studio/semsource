package normalizer

import (
	"fmt"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semstreams/federation"
	"github.com/c360studio/semstreams/message"
)

// Config holds configuration for a Normalizer instance.
type Config struct {
	// Org is the default organization namespace (e.g., "acme").
	// Individual entities may override this by setting Properties["org"] = "public".
	Org string
}

// Normalizer converts RawEntity values from source handlers into normalized
// GraphEntity values with fully-qualified deterministic 6-part entity IDs.
//
// A single Normalizer is safe for concurrent use.
type Normalizer struct {
	cfg Config
}

// New creates a Normalizer with the given configuration.
func New(cfg Config) *Normalizer {
	return &Normalizer{cfg: cfg}
}

// Normalize converts a single RawEntity into a *federation.Entity.
//
// The entity ID is built from:
//
//	{org}.semsource.{domain}.{system}.{entityType}.{instance}
//
// where org defaults to cfg.Org but may be overridden to "public" via
// Properties["org"] to signal an open-source / intrinsic entity.
//
// Properties are converted to triples with Subject=entityID and
// Predicate="{domain}.{entityType}.{key}". Edges are converted to
// relationship triples where Object is the target entity ID.
//
// Required fields: Domain, System, EntityType, Instance.
// Returns an error if any required field is empty or the resulting ID
// fails NATS KV key validation.
func (n *Normalizer) Normalize(raw handler.RawEntity) (*federation.Entity, error) {
	if err := validateRaw(raw); err != nil {
		return nil, fmt.Errorf("normalizer: %w", err)
	}

	org := n.resolveOrg(raw)

	id := BuildEntityID(org, PlatformSemsource, raw.Domain, raw.System, raw.EntityType, raw.Instance)

	if err := ValidateNATSKVKey(id); err != nil {
		return nil, fmt.Errorf("normalizer: built invalid NATS KV key: %w", err)
	}

	triples := n.buildTriples(id, raw)

	entity := &federation.Entity{
		ID:      id,
		Triples: triples,
		Provenance: federation.Provenance{
			SourceType: raw.SourceType,
			SourceID:   raw.System,
			Timestamp:  time.Now().UTC(),
			Handler:    raw.SourceType,
		},
	}

	return entity, nil
}

// NormalizeBatch converts a slice of RawEntity values.
//
// Fail-fast semantics: on the first normalization error the entire batch is
// abandoned and no partial results are returned. This is intentional —
// a partially-normalized batch would produce an inconsistent graph delta.
// The engine logs the error and skips the batch; no entities are upserted or
// emitted for that change event. Handlers are responsible for emitting only
// valid RawEntity values.
func (n *Normalizer) NormalizeBatch(raws []handler.RawEntity) ([]*federation.Entity, error) {
	out := make([]*federation.Entity, 0, len(raws))
	for i, raw := range raws {
		entity, err := n.Normalize(raw)
		if err != nil {
			return nil, fmt.Errorf("normalizer: batch index %d: %w", i, err)
		}
		out = append(out, entity)
	}
	return out, nil
}

// resolveOrg returns the org to use for this entity. If Properties["org"] is
// set to "public", the public namespace is used regardless of the Normalizer's
// configured default org.
func (n *Normalizer) resolveOrg(raw handler.RawEntity) string {
	if v, ok := raw.Properties["org"]; ok {
		if s, ok := v.(string); ok && IsPublicNamespace(s) {
			return "public"
		}
	}
	return n.cfg.Org
}

// buildTriples converts Properties and Edges into triples.
//
// Property triples use Predicate format "{domain}.{entityType}.{key}".
// Relationship triples (from Edges) use Predicate = edge type and
// Object = target entity ID, making them detectable via Triple.IsRelationship().
func (n *Normalizer) buildTriples(entityID string, raw handler.RawEntity) []message.Triple {
	now := time.Now().UTC()

	triples := make([]message.Triple, 0, len(raw.Properties)+len(raw.Edges))

	// Property triples from the Properties map.
	for k, v := range raw.Properties {
		// Skip the internal org override key — it is not a semantic property.
		if k == "org" {
			continue
		}
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  raw.Domain + "." + raw.EntityType + "." + k,
			Object:     v,
			Source:     PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}

	// Relationship triples from Edges.
	if len(raw.Edges) > 0 {
		org := orgFromID(entityID)
		if org != "" {
			for _, re := range raw.Edges {
				targetType := raw.EntityType
				if re.ToType != "" {
					targetType = re.ToType
				}
				toID := BuildEntityID(org, PlatformSemsource, raw.Domain, raw.System, targetType, re.ToHint)
				triples = append(triples, message.Triple{
					Subject:    entityID,
					Predicate:  raw.Domain + "." + raw.EntityType + "." + re.EdgeType,
					Object:     toID,
					Source:     PlatformSemsource,
					Timestamp:  now,
					Confidence: 1.0,
				})
			}
		}
	}

	return triples
}

// orgFromID extracts the first segment (org) from a dot-delimited entity ID.
// Returns empty string if the ID is malformed.
func orgFromID(id string) string {
	if i := len(id); i == 0 {
		return ""
	}
	for i, ch := range id {
		if ch == '.' {
			return id[:i]
		}
	}
	return ""
}

// validateRaw checks that a RawEntity has the minimum fields required to
// construct a valid 6-part entity ID.
func validateRaw(raw handler.RawEntity) error {
	if raw.Domain == "" {
		return fmt.Errorf("RawEntity.Domain is required")
	}
	if raw.System == "" {
		return fmt.Errorf("RawEntity.System is required")
	}
	if raw.EntityType == "" {
		return fmt.Errorf("RawEntity.EntityType is required")
	}
	if raw.Instance == "" {
		return fmt.Errorf("RawEntity.Instance is required")
	}
	return nil
}
