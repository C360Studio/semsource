//go:build qualification && removal

package setup03a

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReplayCheckedStop(t *testing.T) {
	for _, exit := range []int{0, 23} {
		t.Run(fmt.Sprint(exit), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			out := t.TempDir()
			binary := filepath.Join(out, "exit-helper")
			if err := os.WriteFile(binary, []byte(fmt.Sprintf("#!/bin/sh\nexit %d\n", exit)), 0700); err != nil {
				t.Fatal(err)
			}
			h := &harness{t: t, ctx: ctx, out: out, binary: binary}
			h.start()
			defer h.stop(false)
			select {
			case <-h.exited:
			case <-ctx.Done():
				t.Fatal("helper did not exit")
			}
			result, err := replayCheckedStop(h, false)
			if !result.Observed || result.ExitCode != exit {
				t.Fatalf("lost concrete exit: %+v", result)
			}
			if (err != nil) != (exit != 0) {
				t.Fatalf("exit=%d error=%v", exit, err)
			}
		})
	}
}

// This is additive qualification; the original paired corpus and removal probe
// remain untouched. The same private broker persists across observed child death.
func TestRemovalReplayRetirement(t *testing.T) {
	for _, runtimeAdded := range []bool{false, true} {
		name := "original"
		if runtimeAdded {
			name = "runtime_added"
		}
		t.Run(name, func(t *testing.T) {
			p := newReplayQualification(t, runtimeAdded)
			pending := p.remove()
			p.restart("crash_after_durable_intent_observed_exit", true)
			p.settled("retired_producer_absent_after_crash", false)
			p.retainedPass(pending.Binding.Generation)
			p.legacySweep()
			p.unchangedSiblings("sibling_graph_and_exact_content_unchanged")
		})
	}
}

// Every target KV update is observed through the final authoritative revision,
// so a transient old-generation marker cannot hide behind a later fresh seed.
func TestRemovalReplayReaddBeforeRestart(t *testing.T) {
	p := newReplayQualification(t, false)
	stopWatch, err := p.watchTargetStates()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if stopWatch != nil {
			_, err := stopWatch()
			if err != nil {
				t.Error(err)
			}
		}
	}()
	var readd replayRecord
	for turn := 0; turn < 2; turn++ {
		removal := p.remove()
		p.add(fmt.Sprintf("rapid_readd_%d_receipt", turn))
		readd = p.awaitRecord(fmt.Sprintf("rapid_readd_%d_supersedes_old_generation", turn), "reactivate", "pending", removal.Binding.Generation, 8*time.Second)
		p.settled(fmt.Sprintf("rapid_readd_%d_old_boot_unchanged", turn), true)
	}
	p.restart("superseded_readd_checked_exit", false)
	p.settled("readd_same_handle_after_restart", true)
	p.reactivated("readd_current_epoch_exact_manifest", readd, p.targetIDs)
	p.observeMarkers("readded_current_entities_fresh", p.targetIDs, "")
	p.unchangedSiblings("rapid_toggle_siblings_unchanged")
	states, err := stopWatch()
	stopWatch = nil
	raw, marshalErr := json.MarshalIndent(states, "", "  ")
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	p.h.save("target-authority-transitions.json", raw)
	if !p.h.check("zero_old_removal_mutation_across_superseding_readds", time.Nanosecond, func() (any, error) {
		if err != nil {
			return nil, err
		}
		for _, raw := range states {
			var state struct {
				ID      string `json:"id"`
				Triples []struct {
					Predicate string `json:"predicate"`
					Object    any    `json:"object"`
				} `json:"triples"`
			}
			if e := json.Unmarshal(raw, &state); e != nil {
				return string(raw), e
			}
			if state.ID == "" || len(state.Triples) == 0 {
				return string(raw), errors.New("authority observation lacks exact entity/source facts")
			}
			for _, tr := range state.Triples {
				if tr.Predicate == "entity.lifecycle.stale" && tr.Object == "source_removed" {
					return state, errors.New("superseded generation marked a still-admitted entity")
				}
			}
		}
		return map[string]any{"observed_authority_updates": len(states)}, nil
	}) {
		t.FailNow()
	}
}

func TestRemovalReplaySelectiveReactivation(t *testing.T) {
	p := newReplayQualification(t, false)
	removal := p.remove()
	p.restart("removal_graceful_checked_exit", false)
	p.settled("removed_source_absent_before_selective_readd", false)
	p.retainedPass(removal.Binding.Generation)
	p.add("selective_readd_receipt")
	readd := p.awaitRecord("selective_readd_new_generation", "reactivate", "pending", removal.Binding.Generation, 8*time.Second)
	p.settled("selective_readd_deferred", false)
	p.stop("offline_edit_checked_exit", false)
	if err := os.Remove(filepath.Join(p.targetPath, "b.md")); err != nil {
		t.Fatal(err)
	}
	p.h.save("offline-source-change.json", []byte(`{"deleted":"target/b.md","timing":"old process exit observed; before replacement producer starts"}`))
	p.h.start()
	p.settled("selective_readd_admitted", true)
	var current, historical []string
	for _, id := range p.targetIDs {
		if strings.HasSuffix(id, ".a-md") || strings.HasSuffix(id, ".a-md-0000") {
			current = append(current, id)
		} else {
			historical = append(historical, id)
		}
	}
	p.reactivated("selective_current_epoch_A_only_manifest", readd, current)
	p.observeMarkers("current_A_parent_and_passage_fresh", current, "")
	p.observeMarkers("offline_deleted_B_parent_and_passage_retained_stale", historical, "source_removed")
	p.unchangedSiblings("selective_readd_sibling_graph_and_content_unchanged")
}

// A recognized upstream blocker is evidence of an honest pending operation,
// never a substitute for the original positive completion acceptance.
func TestReplayReactivationAcceptanceKeepsConditionalBlockerRed(t *testing.T) {
	before := replayRecord{Operation: "reactivate", Phase: "pending"}
	before.Binding.Generation = 2
	before.Binding.BootEpoch = "old"
	current := before
	current.Binding.BootEpoch = "new"
	if err := replayReactivationComplete(current, before); err == nil {
		t.Fatal("pending reactivation passed positive completion")
	}
	current.Phase = "complete"
	if err := replayReactivationComplete(current, before); err != nil {
		t.Fatalf("same generation new boot completion rejected: %v", err)
	}
	current.Binding.BootEpoch = "old"
	if err := replayReactivationComplete(current, before); err == nil {
		t.Fatal("old epoch passed positive completion")
	}
	current.Binding.BootEpoch = "new"
	current.Binding.Generation++
	if err := replayReactivationComplete(current, before); err == nil {
		t.Fatal("different generation passed positive completion")
	}
}
