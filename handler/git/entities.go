package git

import (
	"strings"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/message"
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
	// Replace characters that would break the 6-part dot-delimited entity ID
	// or are invalid in NATS KV keys.
	r := strings.NewReplacer(
		"@", "-at-",
		".", "-",
		"/", "-",
		" ", "-",
		"\\", "-",
	)
	return r.Replace(s)
}

// --------------------------------------------------------------------------
// Typed entity structs — bypass the normalizer, build triples directly.
// These follow the same pattern as source/ast.CodeEntity.
// --------------------------------------------------------------------------

// CommitEntity is a fully-typed git commit entity that produces triples
// using canonical vocabulary predicates.
type CommitEntity struct {
	ID        string
	SHA       string
	ShortSHA  string
	Author    string
	Subject   string
	System    string
	Org       string
	IndexedAt time.Time

	// Relationship data used to build relationship triples.
	TouchedFiles []string
	AuthorEmail  string
}

// newCommitEntity constructs a CommitEntity and builds its deterministic ID.
func newCommitEntity(org, fullSHA, authorFull, subject, system string, indexedAt time.Time) *CommitEntity {
	sha := shortSHA(fullSHA)
	return &CommitEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "git", system, "commit", sha),
		SHA:       fullSHA,
		ShortSHA:  sha,
		Author:    authorFull,
		Subject:   subject,
		System:    system,
		Org:       org,
		IndexedAt: indexedAt,
	}
}

// Triples converts the CommitEntity to a slice of message.Triple for graph storage.
func (e *CommitEntity) Triples() []message.Triple {
	now := e.IndexedAt
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.GitCommitSHA, Object: e.SHA, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.GitCommitShortSHA, Object: e.ShortSHA, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.GitCommitAuthor, Object: e.Author, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.GitCommitSubject, Object: e.Subject, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
	// File-touch relationship triples.
	for _, f := range e.TouchedFiles {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.GitCommitTouches,
			Object:     f,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	// Authored-by relationship triple: Object is the author entity ID.
	if e.AuthorEmail != "" {
		authorID := entityid.Build(e.Org, entityid.PlatformSemsource, "git", e.System, "author", sanitizeInstance(e.AuthorEmail))
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.GitCommitAuthoredBy,
			Object:     authorID,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	return triples
}

// EntityState converts the CommitEntity to a handler.EntityState for graph publication.
func (e *CommitEntity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:        e.ID,
		Triples:   e.Triples(),
		UpdatedAt: e.IndexedAt,
	}
}

// AuthorEntity is a fully-typed git author entity.
type AuthorEntity struct {
	ID        string
	Name      string
	Email     string
	System    string
	Org       string
	IndexedAt time.Time
}

// newAuthorEntity constructs an AuthorEntity with a deterministic ID.
func newAuthorEntity(org, name, email, system string, indexedAt time.Time) *AuthorEntity {
	return &AuthorEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "git", system, "author", sanitizeInstance(email)),
		Name:      name,
		Email:     email,
		System:    system,
		Org:       org,
		IndexedAt: indexedAt,
	}
}

// Triples converts the AuthorEntity to a slice of message.Triple for graph storage.
func (e *AuthorEntity) Triples() []message.Triple {
	now := e.IndexedAt
	return []message.Triple{
		{Subject: e.ID, Predicate: source.GitAuthorName, Object: e.Name, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.GitAuthorEmail, Object: e.Email, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
}

// EntityState converts the AuthorEntity to a handler.EntityState for graph publication.
func (e *AuthorEntity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:        e.ID,
		Triples:   e.Triples(),
		UpdatedAt: e.IndexedAt,
	}
}

// BranchEntity is a fully-typed git branch entity.
type BranchEntity struct {
	ID         string
	BranchName string
	HeadSHA    string
	System     string
	Org        string
	IndexedAt  time.Time
}

// newBranchEntity constructs a BranchEntity with a deterministic ID.
func newBranchEntity(org, branchName, headSHA, system string, indexedAt time.Time) *BranchEntity {
	return &BranchEntity{
		ID:         entityid.Build(org, entityid.PlatformSemsource, "git", system, "branch", branchName),
		BranchName: branchName,
		HeadSHA:    shortSHA(headSHA),
		System:     system,
		Org:        org,
		IndexedAt:  indexedAt,
	}
}

// Triples converts the BranchEntity to a slice of message.Triple for graph storage.
func (e *BranchEntity) Triples() []message.Triple {
	now := e.IndexedAt
	return []message.Triple{
		{Subject: e.ID, Predicate: source.GitBranchName, Object: e.BranchName, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.GitBranchHeadSHA, Object: e.HeadSHA, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
}

// EntityState converts the BranchEntity to a handler.EntityState for graph publication.
func (e *BranchEntity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:        e.ID,
		Triples:   e.Triples(),
		UpdatedAt: e.IndexedAt,
	}
}
