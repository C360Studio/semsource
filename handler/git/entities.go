package git

import (
	"strings"

	"github.com/c360studio/semsource/handler"
)

// BuildCommitEntity constructs a RawEntity for a git commit.
// Instance is the short SHA (first 7 chars) for deterministic identity.
// Exported for white-box testing.
func BuildCommitEntity(fullSHA, authorFull, subject, system string) handler.RawEntity {
	sha := shortSHA(fullSHA)

	return handler.RawEntity{
		SourceType: handler.SourceTypeGit,
		Domain:     handler.DomainGit,
		System:     system,
		EntityType: "commit",
		Instance:   sha,
		Properties: map[string]any{
			"sha":       fullSHA,
			"short_sha": sha,
			"author":    authorFull,
			"subject":   subject,
		},
	}
}

// BuildAuthorEntity constructs a RawEntity for a git commit author.
// Instance is derived from the author's email for deterministic identity.
// Exported for white-box testing.
func BuildAuthorEntity(name, email, system string) handler.RawEntity {
	// Use email as the instance identifier — stable and unique per person.
	instance := sanitizeInstance(email)

	return handler.RawEntity{
		SourceType: handler.SourceTypeGit,
		Domain:     handler.DomainGit,
		System:     system,
		EntityType: "author",
		Instance:   instance,
		Properties: map[string]any{
			"name":  name,
			"email": email,
		},
	}
}

// BuildBranchEntity constructs a RawEntity for a git branch.
// Instance is the branch name.
// Exported for white-box testing.
func BuildBranchEntity(branchName, headSHA, system string) handler.RawEntity {
	sha := shortSHA(headSHA)

	return handler.RawEntity{
		SourceType: handler.SourceTypeGit,
		Domain:     handler.DomainGit,
		System:     system,
		EntityType: "branch",
		Instance:   branchName,
		Properties: map[string]any{
			"name":     branchName,
			"head_sha": sha,
		},
	}
}

// sanitizeInstance makes a string safe for use as an entity instance field.
// Replaces characters that would be invalid in a NATS KV key segment.
func sanitizeInstance(s string) string {
	// Replace @, /, spaces with hyphens.
	r := strings.NewReplacer(
		"@", "-at-",
		"/", "-",
		" ", "-",
		"\\", "-",
	)
	return r.Replace(s)
}
