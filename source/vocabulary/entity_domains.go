package source

import semtypes "github.com/c360studio/semstreams/pkg/types"

// EntityDomainDelegations declares the source taxonomies minted by SemSource.
// This offline audit declaration grants no runtime subject authority.
func EntityDomainDelegations() []semtypes.EntityDomainDelegation {
	return []semtypes.EntityDomainDelegation{
		{Producer: "semsource", Domain: "code"},
		{Producer: "semsource", Domain: "golang"},
		{Producer: "semsource", Domain: "typescript"},
		{Producer: "semsource", Domain: "javascript"},
		{Producer: "semsource", Domain: "java"},
		{Producer: "semsource", Domain: "python"},
		{Producer: "semsource", Domain: "svelte"},
		{Producer: "semsource", Domain: "c"},
		{Producer: "semsource", Domain: "cpp"},
		{Producer: "semsource", Domain: "git"},
		{Producer: "semsource", Domain: "web"},
		{Producer: "semsource", Domain: "config"},
		{Producer: "semsource", Domain: "media"},
	}
}
