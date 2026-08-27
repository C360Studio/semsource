package sourcespawn

import (
	"testing"

	"github.com/c360studio/semsource/config"
)

// TestEverySourceTypeConfigAcceptsIsSpawnable closes a gap that predates the
// object-store source.
//
// config.validSourceTypes and buildSpecs' type switch are two independent
// lists, and until now nothing checked that they agreed. A type in the first
// and missing from the second passes `semsource validate` and then fails at
// `semsource run`. It fails loudly — a typed CodeUnsupportedType, not silent
// degradation — but validate has still made a promise it had no way to keep,
// and runtime-configuration states that promise as "validate-pass implies a
// startable configuration".
//
// The assertion runs in ONE direction only, config -> spawn. The reverse, and
// the config -> CLI direction, are deliberately not asserted: a type valid in
// semsource.json with no `semsource add` subcommand is a UX gap, not a
// correctness bug, and asserting it would force CLI surface nobody asked for.
//
// This test lands BEFORE the object-store type is added, so the guard is
// proven green against the nine types that exist today rather than introduced
// alongside a tenth and assumed to work.
func TestEverySourceTypeConfigAcceptsIsSpawnable(t *testing.T) {
	types := config.SourceTypes()
	if len(types) == 0 {
		t.Fatal("config accepts no source types — the guard would pass vacuously")
	}

	for _, sourceType := range types {
		t.Run(sourceType, func(t *testing.T) {
			src := minimalEntry(t, sourceType)

			// The premise: this entry is one `semsource validate` accepts. If
			// it does not, the spawn assertion below proves nothing about the
			// promise being kept.
			if err := src.Validate(); err != nil {
				t.Fatalf("fixture for %q does not pass validation: %v", sourceType, err)
			}

			specs, err := buildSpecs(t.Context(), src, Options{
				Org:          "acme",
				WorkspaceDir: t.TempDir(),
			})
			if CodeOf(err) == CodeUnsupportedType {
				t.Fatalf("config accepts source type %q but buildSpecs cannot construct it — "+
					"a semsource.json using it passes validate and fails at run", sourceType)
			}
			if err != nil {
				t.Fatalf("buildSpecs(%q): %v", sourceType, err)
			}
			if len(specs) == 0 {
				t.Fatalf("source type %q produced no component specs", sourceType)
			}
			for _, spec := range specs {
				if spec.factoryName == "" {
					t.Errorf("source type %q produced a spec with no factory", sourceType)
				}
				if spec.instanceName == "" {
					t.Errorf("source type %q produced a spec with no instance name", sourceType)
				}
			}
		})
	}
}

// minimalEntry returns the smallest valid SourceEntry for a type.
//
// Branches and paths are local and explicit so nothing here reaches the
// network: an empty Branch would send the git leaf to `git ls-remote` to
// resolve the remote's default.
//
// The default case fails rather than returning a zero entry. A new source type
// added without a fixture must break this test loudly — silently exercising an
// empty entry would leave the guard green while checking nothing.
func minimalEntry(t *testing.T, sourceType string) config.SourceEntry {
	t.Helper()

	switch sourceType {
	case "git":
		return config.SourceEntry{Type: "git", Path: t.TempDir(), Branch: "main"}
	case "repo":
		return config.SourceEntry{Type: "repo", Path: t.TempDir(), Branch: "main"}
	case "ast":
		return config.SourceEntry{Type: "ast", Path: t.TempDir(), Language: "go"}
	case "docs":
		return config.SourceEntry{Type: "docs", Paths: []string{t.TempDir()}}
	case "config":
		return config.SourceEntry{Type: "config", Paths: []string{t.TempDir()}}
	case "url":
		return config.SourceEntry{Type: "url", URLs: []string{"https://example.com/doc"}}
	case "s3":
		return config.SourceEntry{Type: "s3", Bucket: "artifacts", Prefix: "reports/"}
	case "image", "video", "audio":
		return config.SourceEntry{Type: sourceType, Paths: []string{t.TempDir()}}
	default:
		t.Fatalf("no fixture for source type %q — add one alongside the type itself, "+
			"or this guard silently stops covering it", sourceType)
		return config.SourceEntry{}
	}
}

// TestObjectStoreSpecCarriesItsFactoryAndIdentity pins what the s3 case
// produces, since the invariant test above only asserts that it produces
// something.
func TestObjectStoreSpecCarriesItsFactoryAndIdentity(t *testing.T) {
	src := config.SourceEntry{Type: "s3", Bucket: "artifacts", Prefix: "reports/", Watch: true}

	specs, err := buildSpecs(t.Context(), src, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("buildSpecs: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("produced %d specs, want 1", len(specs))
	}

	spec := specs[0]
	if spec.factoryName != "objectstore-source" {
		t.Errorf("factory = %q, want objectstore-source", spec.factoryName)
	}
	if spec.sourceType != "s3" {
		t.Errorf("sourceType = %q", spec.sourceType)
	}
	if spec.compCfg["bucket"] != "artifacts" || spec.compCfg["prefix"] != "reports/" {
		t.Errorf("config lost the target: %+v", spec.compCfg)
	}
	if spec.compCfg["instance_name"] != spec.instanceName {
		t.Error("the component's instance_name disagrees with its spec's")
	}

	// No credential passes through the spawn path. The component reads them
	// from the environment; anything here would reach the watched config KV.
	for key := range spec.compCfg {
		switch key {
		case "access_key_id", "secret_access_key", "credentials", "aws_access_key_id":
			t.Errorf("spawn config carries a credential field %q", key)
		}
	}
}

// TestObjectStoreInstanceNamesScopeByPrefix keeps two prefixes of one bucket
// from collapsing onto a single component. They are separate sources, and a
// shared instance name would mean the second registration silently replaced
// the first.
func TestObjectStoreInstanceNamesScopeByPrefix(t *testing.T) {
	reports, err := buildSpecs(t.Context(),
		config.SourceEntry{Type: "s3", Bucket: "artifacts", Prefix: "reports/"}, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("buildSpecs: %v", err)
	}
	memos, err := buildSpecs(t.Context(),
		config.SourceEntry{Type: "s3", Bucket: "artifacts", Prefix: "memos/"}, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("buildSpecs: %v", err)
	}

	if reports[0].instanceName == memos[0].instanceName {
		t.Errorf("two prefixes of one bucket share the instance name %q", reports[0].instanceName)
	}

	// Deterministic: the same entry must name the same component every time,
	// or a restart orphans the previous one.
	again, err := buildSpecs(t.Context(),
		config.SourceEntry{Type: "s3", Bucket: "artifacts", Prefix: "reports/"}, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("buildSpecs: %v", err)
	}
	if again[0].instanceName != reports[0].instanceName {
		t.Errorf("instance name is not deterministic: %q then %q", reports[0].instanceName, again[0].instanceName)
	}
}
