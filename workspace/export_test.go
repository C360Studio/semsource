package workspace

import "context"

// ListSubmodulesCapped exposes the depth-capped inventory to external tests
// so the BeyondCap contract is testable without building an 11-deep chain.
func ListSubmodulesCapped(ctx context.Context, repoPath string, maxDepth int) (*SubmoduleInventory, error) {
	return listSubmodules(ctx, repoPath, maxDepth)
}

// ResolveSubmoduleURL exposes relative-URL resolution to external tests.
var ResolveSubmoduleURL = resolveSubmoduleURL
