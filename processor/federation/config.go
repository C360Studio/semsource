// Package federation provides merge policy logic for the FederationProcessor.
// It applies the SemSource federation rules (spec S7.4) to incoming GraphEvents,
// enforcing namespace sovereignty and edge union semantics.
package federation

import (
	"fmt"
)

// MergePolicy controls how incoming entities are merged against the local graph.
type MergePolicy string

const (
	// MergePolicyStandard applies the rules from spec S7.4:
	//   - public.* merges unconditionally
	//   - {org}.* merges only if org matches LocalNamespace
	//   - cross-org entities are rejected
	//   - edges use union semantics
	//   - provenance is always appended
	MergePolicyStandard MergePolicy = "standard"
)

// validMergePolicies is the set of recognized merge policy values.
var validMergePolicies = map[MergePolicy]bool{
	MergePolicyStandard: true,
}

// Config holds configuration for a FederationProcessor or Merger.
type Config struct {
	// LocalNamespace is the org namespace this processor is authoritative for
	// (e.g. "acme"). Entities from other orgs are rejected unless they are
	// in the "public" namespace.
	LocalNamespace string `json:"local_namespace" schema:"type:string,description:Org namespace this processor is authoritative for (e.g. acme or public),category:basic"`

	// MergePolicy controls the entity merge strategy. Valid values: "standard".
	MergePolicy MergePolicy `json:"merge_policy" schema:"type:string,description:Entity merge strategy,category:basic,enum:standard,default:standard"`
}

// Validate checks that Config contains all required and valid field values.
func (c Config) Validate() error {
	if c.LocalNamespace == "" {
		return fmt.Errorf("federation config: local_namespace is required")
	}
	if c.MergePolicy == "" {
		return fmt.Errorf("federation config: merge_policy is required")
	}
	if !validMergePolicies[c.MergePolicy] {
		return fmt.Errorf("federation config: unknown merge_policy %q (valid: standard)", c.MergePolicy)
	}
	return nil
}

// DefaultConfig returns a Config with sensible defaults.
// LocalNamespace is set to "public" as a safe starting point; callers should
// override it with the actual org namespace.
func DefaultConfig() Config {
	return Config{
		LocalNamespace: "public",
		MergePolicy:    MergePolicyStandard,
	}
}
