package c

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

// One-off measurement backing design D4 / task 4.1 of call-graph-completeness:
// how often does one C function NAME have definitions in more than one file,
// as seen by OUR parser? Run with C_MEASURE_CORPUS=<dir>; skipped otherwise.
func TestMeasureCrossTUCollisions(t *testing.T) {
	corpus := os.Getenv("C_MEASURE_CORPUS")
	if corpus == "" {
		t.Skip("set C_MEASURE_CORPUS to run the measurement")
	}
	p := NewParser("acme", "measure", corpus)
	defsByName := make(map[string]map[string]bool) // name -> set of files
	total := 0
	err := filepath.Walk(corpus, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".c" {
			return nil
		}
		res, perr := p.ParseFile(context.Background(), path)
		if perr != nil {
			return nil
		}
		for _, e := range res.Entities {
			if e.Type != ast.TypeFunction {
				continue
			}
			if defsByName[e.Name] == nil {
				defsByName[e.Name] = make(map[string]bool)
			}
			defsByName[e.Name][e.Path] = true
			total++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	names, colliding := 0, 0
	for _, files := range defsByName {
		names++
		if len(files) > 1 {
			colliding++
		}
	}
	fmt.Printf("C collision measurement: %d function defs, %d distinct names, %d names defined in >1 file (%.2f%%)\n",
		total, names, colliding, 100*float64(colliding)/float64(names))
}
