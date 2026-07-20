package config

import (
	"strings"
	"testing"

	"github.com/c360studio/semsource/entityid"
)

// TestValidateNamespace pins the segment contract at the earliest surface: a
// namespace that would make every published entity invalid must fail here,
// not at runtime (audit 2026-07-19: "acme.io" passed validate and produced a
// permanently empty graph).
func TestValidateNamespace(t *testing.T) {
	invalid := []string{"acme.io", "acme corp", "-acme", "_acme", "café", "a/b", ""}
	for _, ns := range invalid {
		if err := ValidateNamespace(ns); err == nil {
			t.Errorf("ValidateNamespace(%q) = nil, want error", ns)
		}
	}
	valid := []string{"acme", "c360", "Acme-Corp", "org_1", "a"}
	for _, ns := range valid {
		if err := ValidateNamespace(ns); err != nil {
			t.Errorf("ValidateNamespace(%q) = %v, want nil", ns, err)
		}
	}
}

// TestValidateNamespace_RejectsOverlongOrg closes the last route to Build's
// truncation backstop. Org is the only entity-ID segment this codebase neither
// derives nor caps — it comes straight from config — so an over-long one is
// charset-valid yet leaves too little budget for real repo and symbol names.
// The failure must land at startup, not as rejected entities during ingest:
// an over-long org breaks every entity in the deployment, not one.
func TestValidateNamespace_RejectsOverlongOrg(t *testing.T) {
	overlong := strings.Repeat("a", entityid.MaxOrgLen+1)
	if err := ValidateNamespace(overlong); err == nil {
		t.Errorf("ValidateNamespace accepted a %d-byte org", len(overlong))
	}
	atLimit := strings.Repeat("a", entityid.MaxOrgLen)
	if err := ValidateNamespace(atLimit); err != nil {
		t.Errorf("ValidateNamespace rejected an org at the limit (%d bytes): %v", len(atLimit), err)
	}
}

// TestConfigValidate_RejectsInvalidNamespace pins the loader path: run,
// validate, and add all load through Validate, so a dotted org fails before
// any component starts, with actionable guidance.
func TestConfigValidate_RejectsInvalidNamespace(t *testing.T) {
	cfg := &Config{
		Namespace: "acme.io",
		Sources:   []SourceEntry{{Type: "docs", Paths: []string{"./docs"}}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate accepted a dotted namespace")
	}
	if !strings.Contains(err.Error(), "acme.io") || !strings.Contains(err.Error(), "a-zA-Z0-9") {
		t.Errorf("error lacks value/alphabet guidance: %v", err)
	}
}
