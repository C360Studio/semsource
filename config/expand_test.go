package config

import (
	"context"
	"testing"
)

func TestExpandRepoSources_ExpandsSingleRepo(t *testing.T) {
	sources := []SourceEntry{
		{Type: "repo", URL: "https://github.com/opensensorhub/osh-core", Language: "java", Watch: true, Branch: "master"},
	}
	result, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expanded := result.Sources
	if len(expanded) != 4 {
		t.Fatalf("expected 4 expanded sources, got %d", len(expanded))
	}
	// First must be git
	if expanded[0].Type != "git" {
		t.Errorf("first source type = %q, want git", expanded[0].Type)
	}
	if expanded[0].URL != "https://github.com/opensensorhub/osh-core" {
		t.Errorf("git URL = %q", expanded[0].URL)
	}
	if expanded[0].Branch != "master" {
		t.Errorf("git branch = %q, want master", expanded[0].Branch)
	}
	// Second must be ast with language
	if expanded[1].Type != "ast" {
		t.Errorf("second source type = %q, want ast", expanded[1].Type)
	}
	if expanded[1].Language != "java" {
		t.Errorf("ast language = %q, want java", expanded[1].Language)
	}
	if expanded[1].Path == "" {
		t.Error("ast path must not be empty")
	}
	// Third must be docs
	if expanded[2].Type != "docs" {
		t.Errorf("third source type = %q, want docs", expanded[2].Type)
	}
	// Fourth must be config
	if expanded[3].Type != "config" {
		t.Errorf("fourth source type = %q, want config", expanded[3].Type)
	}
	// All should have watch=true
	for i, s := range expanded {
		if !s.Watch {
			t.Errorf("expanded[%d].Watch = false, want true", i)
		}
	}
	// No branch watchers for single-branch mode
	if len(result.Watchers) != 0 {
		t.Errorf("expected 0 watchers, got %d", len(result.Watchers))
	}
}

func TestExpandRepoSources_PreservesNonRepoSources(t *testing.T) {
	sources := []SourceEntry{
		{Type: "ast", Path: "/some/path"},
		{Type: "git", URL: "https://example.com/repo"},
	}
	result, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expanded := result.Sources
	if len(expanded) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(expanded))
	}
	if expanded[0].Type != "ast" || expanded[1].Type != "git" {
		t.Error("non-repo sources should pass through unchanged")
	}
}

func TestExpandRepoSources_MixedSources(t *testing.T) {
	sources := []SourceEntry{
		{Type: "url", URLs: []string{"https://example.com"}},
		{Type: "repo", URL: "https://github.com/example/repo"},
		{Type: "ast", Path: "."},
	}
	result, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expanded := result.Sources
	// 1 (url) + 4 (repo expanded) + 1 (ast) = 6
	if len(expanded) != 6 {
		t.Fatalf("expected 6 sources, got %d", len(expanded))
	}
	if expanded[0].Type != "url" {
		t.Errorf("first should be url, got %q", expanded[0].Type)
	}
	if expanded[1].Type != "git" {
		t.Errorf("second should be git (from repo expansion), got %q", expanded[1].Type)
	}
	if expanded[5].Type != "ast" {
		t.Errorf("last should be ast, got %q", expanded[5].Type)
	}
}

func TestExpandRepoSources_RequiresURLOrPath(t *testing.T) {
	sources := []SourceEntry{
		{Type: "repo"},
	}
	_, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err == nil {
		t.Fatal("expected error for repo without URL or path")
	}
}

func TestExpandRepoSources_LanguagePropagation(t *testing.T) {
	sources := []SourceEntry{
		{Type: "repo", URL: "https://github.com/example/repo", Language: "python"},
	}
	result, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range result.Sources {
		if s.Type == "ast" {
			if s.Language != "python" {
				t.Errorf("ast language = %q, want python", s.Language)
			}
			return
		}
	}
	t.Error("no ast entry found in expanded sources")
}

func TestExpandRepoSources_LocalRepoPath(t *testing.T) {
	sources := []SourceEntry{
		{Type: "repo", Path: "/home/user/projects/my-app", Watch: true},
	}
	result, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expanded := result.Sources
	if len(expanded) != 4 {
		t.Fatalf("expected 4 expanded sources, got %d", len(expanded))
	}
	if expanded[0].Path != "/home/user/projects/my-app" {
		t.Errorf("git path = %q, want /home/user/projects/my-app", expanded[0].Path)
	}
	if expanded[1].Path != "/home/user/projects/my-app" {
		t.Errorf("ast path = %q, want /home/user/projects/my-app", expanded[1].Path)
	}
}
