package source_test

import (
	"bytes"
	goast "go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	_ "github.com/c360studio/semsource/source/ast"
	semvocabulary "github.com/c360studio/semstreams/vocabulary"
	"gopkg.in/yaml.v3"
)

const predicateLedgerPath = "openspec/changes/archive/" +
	"2026-07-18-migrate-semstreams-beta-148-contracts/predicate-rename-ledger.yaml"

var predicateDeclarationFiles = []string{
	"source/ast/predicates.go",
	"source/vocabulary/config.go",
	"source/vocabulary/convention.go",
	"source/vocabulary/git.go",
	"source/vocabulary/media.go",
	"source/vocabulary/predicates.go",
}

// These are the only behavior roots in the beta.148 source-vocabulary cutover.
// The reviewed ledger is deliberately outside them. Parsing without comments
// also permits explanatory negative-grammar comments without permitting a
// retired identity in an executable Go string literal.
var predicateBehaviorRoots = []string{"handler", "processor", "source", "internal"}

// beta148RetiredSymbols are ledger rows whose symbol has since been deleted as
// dead vocabulary: every one carries producer: none and exact_query: none, so
// nothing ever emitted or queried them. The ledger is an archived record of the
// beta.148 rename and stays byte-for-byte intact — the rename genuinely
// happened. What changes is the assertion: for these rows the contract is
// inverted from "still declared and registered" to "provably gone", so a
// deletion cannot silently regress into a re-registration.
var beta148RetiredSymbols = map[string]struct{}{
	"DocAppliesTo":       {},
	"DocRelatedDomains":  {},
	"RepoAutoPull":       {},
	"RepoEntityCount":    {},
	"RepoLastCommit":     {},
	"RepoLastIndexed":    {},
	"RepoPullInterval":   {},
	"SourceAddedAt":      {},
	"SourceAddedBy":      {},
	"WebAnalysisSkipped": {},
	"WebAppliesTo":       {},
	"WebAutoRefresh":     {},
	"WebChunkCount":      {},
	"WebChunkIndex":      {},
	"WebLastFetched":     {},
	"WebRefreshInterval": {},
	"WebRelatedDomains":  {},
	"WebSemanticDomain":  {},
}

type beta148PredicateLedger struct {
	Version  int                         `yaml:"version"`
	RowCount int                         `yaml:"row_count"`
	Rows     []beta148PredicateLedgerRow `yaml:"rows"`
}

type beta148PredicateLedgerRow struct {
	OldIdentity     string   `yaml:"old_identity"`
	CanonicalTarget string   `yaml:"canonical_target"`
	Symbol          string   `yaml:"symbol"`
	Registration    string   `yaml:"registration"`
	Producer        []string `yaml:"producer"`
	ExactQuery      []string `yaml:"exact_query"`
}

func TestBeta148PredicateMigrationContract(t *testing.T) {
	root := beta148RepositoryRoot(t)
	ledger := loadBeta148PredicateLedger(t, root)
	if ledger.Version != 1 {
		t.Errorf("ledger version = %d, want 1", ledger.Version)
	}
	if ledger.RowCount != 92 || len(ledger.Rows) != 92 {
		t.Fatalf("ledger rows = header %d decoded %d, want 92", ledger.RowCount, len(ledger.Rows))
	}

	declarations := beta148PredicateDeclarations(t, root)
	targetOwners := make(map[string]string, len(ledger.Rows))
	additionSymbols := map[string]struct{}{
		"DocBodyStore": {},
		"DocBodyKey":   {},
	}
	replacements := 0
	additions := 0

	for _, row := range ledger.Rows {
		if _, addition := additionSymbols[row.Symbol]; addition {
			additions++
		} else {
			replacements++
		}
		if prior, duplicate := targetOwners[row.CanonicalTarget]; duplicate {
			t.Errorf("canonical target %q is shared by %s and %s", row.CanonicalTarget, prior, row.Symbol)
		}
		targetOwners[row.CanonicalTarget] = row.Symbol
		if _, err := semvocabulary.ParsePredicate(row.CanonicalTarget); err != nil {
			t.Errorf("%s target %q rejected by beta.148 parser: %v", row.Symbol, row.CanonicalTarget, err)
		}
		if _, retired := beta148RetiredSymbols[row.Symbol]; retired {
			if got, declared := declarations[row.Symbol]; declared {
				t.Errorf("retired symbol %s is declared again as %q", row.Symbol, got)
			}
			if metadata := semvocabulary.GetPredicateMetadata(row.CanonicalTarget); metadata != nil {
				t.Errorf("retired symbol %s target %q is registered again", row.Symbol, row.CanonicalTarget)
			}
			continue
		}
		if metadata := semvocabulary.GetPredicateMetadata(row.CanonicalTarget); metadata == nil {
			t.Errorf("%s target %q is not registered", row.Symbol, row.CanonicalTarget)
		}
		if got := declarations[row.Symbol]; got != row.CanonicalTarget {
			t.Errorf("%s declaration = %q, want %q", row.Symbol, got, row.CanonicalTarget)
		}
		if strings.HasPrefix(row.Registration, "MISSING:") {
			t.Errorf("%s still claims a missing registration: %s", row.Symbol, row.Registration)
		}
		assertBeta148SurfacesAgree(t, root, row)
	}

	if len(beta148RetiredSymbols) != 18 {
		t.Errorf("retired symbol count = %d, want 18", len(beta148RetiredSymbols))
	}
	if replacements != 90 || additions != 2 {
		t.Errorf("migration shape = %d replacements + %d additions, want 90 + 2", replacements, additions)
	}
	if len(targetOwners) != 92 {
		t.Errorf("unique canonical targets = %d, want 92", len(targetOwners))
	}
	assertNoRetiredBehaviorLiterals(t, root, ledger.Rows)
}

func beta148RepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve beta.148 contract test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func loadBeta148PredicateLedger(t *testing.T, root string) beta148PredicateLedger {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, predicateLedgerPath))
	if err != nil {
		t.Fatal(err)
	}
	var ledger beta148PredicateLedger
	if err := yaml.Unmarshal(data, &ledger); err != nil {
		t.Fatal(err)
	}
	return ledger
}

func beta148PredicateDeclarations(t *testing.T, root string) map[string]string {
	t.Helper()
	declarations := make(map[string]string)
	for _, relativePath := range predicateDeclarationFiles {
		path := filepath.Join(root, relativePath)
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			general, ok := declaration.(*goast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				values, ok := spec.(*goast.ValueSpec)
				if !ok || len(values.Values) == 0 {
					continue
				}
				for index, name := range values.Names {
					valueIndex := index
					if len(values.Values) == 1 {
						valueIndex = 0
					}
					if valueIndex >= len(values.Values) {
						continue
					}
					literal, ok := values.Values[valueIndex].(*goast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					value, err := strconv.Unquote(literal.Value)
					if err != nil {
						t.Fatal(err)
					}
					declarations[name.Name] = value
				}
			}
		}
	}
	return declarations
}

func assertBeta148SurfacesAgree(t *testing.T, root string, row beta148PredicateLedgerRow) {
	t.Helper()
	identifier := regexp.MustCompile(`\b` + regexp.QuoteMeta(row.Symbol) + `\b`)
	for _, relativePath := range append(append([]string{}, row.Producer...), row.ExactQuery...) {
		if relativePath == "none" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, relativePath))
		if err != nil {
			t.Errorf("%s surface %s: %v", row.Symbol, relativePath, err)
			continue
		}
		if bytes.Contains(data, []byte(row.OldIdentity)) {
			t.Errorf("%s surface %s retains %q", row.Symbol, relativePath, row.OldIdentity)
		}
		if !identifier.Match(data) && !bytes.Contains(data, []byte(row.CanonicalTarget)) {
			t.Errorf("%s surface %s contains neither the symbol nor canonical target", row.Symbol, relativePath)
		}
	}
}

func assertNoRetiredBehaviorLiterals(t *testing.T, root string, rows []beta148PredicateLedgerRow) {
	t.Helper()
	retired := make([]string, 0, len(rows))
	for _, row := range rows {
		retired = append(retired, row.OldIdentity)
	}
	for _, relativeRoot := range predicateBehaviorRoots {
		err := filepath.WalkDir(filepath.Join(root, relativeRoot), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			goast.Inspect(parsed, func(node goast.Node) bool {
				literal, ok := node.(*goast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					return true
				}
				for _, oldIdentity := range retired {
					if strings.Contains(value, oldIdentity) {
						relativePath, _ := filepath.Rel(root, path)
						t.Errorf("retired behavior literal %q in %s", oldIdentity, relativePath)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
