package git

import (
	"strings"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semstreams/message"
)

// BuildCommitEntity constructs a RawEntity for a git commit.
// Instance is the short SHA (first 7 chars) for deterministic identity.
// Exported for white-box testing.
func BuildCommitEntity(fullSHA, authorFull, subject, system string) handler.RawEntity {
	sha := shortSHA(fullSHA)
	now := time.Now().UTC()

	triples := []message.Triple{
		{Subject: sha, Predicate: "git.commit.sha", Object: fullSHA, Source: "git", Timestamp: now, Confidence: 1.0},
		{Subject: sha, Predicate: "git.commit.short_sha", Object: sha, Source: "git", Timestamp: now, Confidence: 1.0},
		{Subject: sha, Predicate: "git.commit.author", Object: authorFull, Source: "git", Timestamp: now, Confidence: 1.0},
		{Subject: sha, Predicate: "git.commit.subject", Object: subject, Source: "git", Timestamp: now, Confidence: 1.0},
	}

	return handler.RawEntity{
		SourceType: handler.SourceTypeGit,
		Domain:     handler.DomainGit,
		System:     system,
		EntityType: "commit",
		Instance:   sha,
		Triples:    triples,
	}
}

// BuildAuthorEntity constructs a RawEntity for a git commit author.
// Instance is derived from the author's email for deterministic identity.
// Exported for white-box testing.
func BuildAuthorEntity(name, email, system string) handler.RawEntity {
	// Use email as the instance identifier — stable and unique per person.
	instance := sanitizeInstance(email)
	now := time.Now().UTC()

	triples := []message.Triple{
		{Subject: instance, Predicate: "git.author.name", Object: name, Source: "git", Timestamp: now, Confidence: 1.0},
		{Subject: instance, Predicate: "git.author.email", Object: email, Source: "git", Timestamp: now, Confidence: 1.0},
	}

	return handler.RawEntity{
		SourceType: handler.SourceTypeGit,
		Domain:     handler.DomainGit,
		System:     system,
		EntityType: "author",
		Instance:   instance,
		Triples:    triples,
	}
}

// BuildBranchEntity constructs a RawEntity for a git branch.
// Instance is the branch name.
// Exported for white-box testing.
func BuildBranchEntity(branchName, headSHA, system string) handler.RawEntity {
	sha := shortSHA(headSHA)
	now := time.Now().UTC()

	triples := []message.Triple{
		{Subject: branchName, Predicate: "git.branch.name", Object: branchName, Source: "git", Timestamp: now, Confidence: 1.0},
		{Subject: branchName, Predicate: "git.branch.head_sha", Object: sha, Source: "git", Timestamp: now, Confidence: 1.0},
	}

	return handler.RawEntity{
		SourceType: handler.SourceTypeGit,
		Domain:     handler.DomainGit,
		System:     system,
		EntityType: "branch",
		Instance:   branchName,
		Triples:    triples,
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
