//go:build qualification && removal

package setup03a

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// TestDesiredSourceLifecycleWithoutRecovery qualifies the changed public contract
// separately from the unchanged historical source-removal/freshness workload.
// Boundaries are checked graceful application replacement with the same broker;
// this test does not inject a crash, ambiguous commit, or broker restart.
func TestDesiredSourceLifecycleWithoutRecovery(t *testing.T) {
	for _, runtimeAdded := range []bool{false, true} {
		name := "original"
		if runtimeAdded {
			name = "runtime_added"
		}
		t.Run(name, func(t *testing.T) {
			p := newReplayQualification(t, runtimeAdded)
			p.h.result.Profile = "bm25-desired-lifecycle-correction"
			p.h.save("changed-contract.json", []byte(`{"contract":"desired-only source changes; projection unavailable","boundary":"checked graceful application restart; same broker","automatic_source_removal_qualified":false,"positive_reactivation_qualified":false,"providers":"none; frozen BM25 configuration","http_auth":"trusted-network fixture; no token configured"}`))
			p.correctionNoJournal("fresh_boot_has_no_private_lifecycle_bucket")
			envelope := p.correctionDesired("initial_complete_desired_envelope_captured", true, "")
			p.correctionCaptureTargets()
			p.correctionUnavailable("initial")
			p.correctionChange(false)
			p.settled("remove_keeps_current_producer_admitted", true)
			p.correctionDesired("disabled_envelope_retained_before_restart", false, envelope)
			p.correctionRepeatedRemove()
			p.restart("desired_remove_checked_exit", false)
			p.settled("disabled_producer_absent_after_restart", false)
			p.correctionDesired("disabled_envelope_survives_restart", false, envelope)
			p.correctionNoJournal("removed_boot_has_no_private_lifecycle_bucket")
			p.unchangedSiblings("removal_retains_exact_target_sibling_facts_and_content")
			p.correctionUnavailable("removed")
			p.correctionChange(true)
			p.settled("readd_waits_for_next_boot", false)
			p.correctionDesired("readd_retains_enabled_desired_envelope", true, envelope)
			p.restart("desired_readd_checked_exit", false)
			p.settled("readded_producer_admitted_after_restart", true)
			p.correctionDesired("enabled_envelope_survives_restart", true, envelope)
			p.correctionNoJournal("readded_boot_has_no_private_lifecycle_bucket")
			p.unchangedSiblings("readd_preserves_exact_target_sibling_facts_and_content")
		})
	}
}

func (p *replayQualification) correctionNoJournal(name string) {
	p.h.check(name, time.Nanosecond, func() (any, error) {
		ctx, cancel := context.WithTimeout(p.h.ctx, 3*time.Second)
		defer cancel()
		_, err := p.h.js.KeyValue(ctx, "SEMSOURCE_SOURCE_LIFECYCLE")
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			return map[string]any{"bucket": "SEMSOURCE_SOURCE_LIFECYCLE", "present": false}, nil
		}
		if err != nil {
			return nil, err
		}
		return nil, errors.New("fresh runtime created private lifecycle storage")
	})
}

func (p *replayQualification) correctionCaptureTargets() {
	if !p.h.check("target_facts_and_exact_content_captured", 20*time.Second, func() (any, error) {
		store, err := p.h.js.ObjectStore(p.h.ctx, "CONTENT")
		if err != nil {
			return nil, err
		}
		bodies := 0
		for _, id := range p.targetIDs {
			entity, err := p.h.entity(id)
			if err != nil {
				return entity, err
			}
			p.siblings[id] = replaySourceFacts(entity)
			if key, ok := value(entity, "source.doc.body-key").(string); ok {
				body, err := store.GetBytes(p.h.ctx, key)
				if err != nil {
					return nil, err
				}
				p.content[key] = string(body)
				bodies++
			}
		}
		if bodies != 2 {
			return nil, fmt.Errorf("target passage bodies=%d, want 2", bodies)
		}
		return map[string]any{"target_ids": p.targetIDs, "target_bodies": bodies}, nil
	}) {
		p.h.t.FailNow()
	}
}

func (p *replayQualification) correctionChange(add bool) {
	subject, name := "graph.ingest.remove.setup03a", "desired_remove_receipt"
	input := map[string]any{"instance_name": replayTargetHandle, "provenance": map[string]string{"actor": "desired-lifecycle-correction"}}
	if add {
		subject, name = "graph.ingest.add.setup03a", "desired_readd_receipt"
		input = map[string]any{"source": p.targetSource(), "provenance": map[string]string{"actor": "desired-lifecycle-correction"}}
	}
	raw, requestErr := p.h.rpc(subject, input)
	p.h.save(name+".json", raw)
	if !p.h.check(name, time.Nanosecond, func() (any, error) {
		if requestErr != nil {
			return json.RawMessage(raw), requestErr
		}
		var reply map[string]any
		if err := json.Unmarshal(raw, &reply); err != nil {
			return nil, err
		}
		if reply["error"] != nil || reply["desired_changed"] != true || reply["runtime_changed"] != false || reply["restart_required"] != true || reply["projection_status"] != "unavailable" {
			return reply, errors.New("desired change omitted truthful activation/projection result")
		}
		if _, exists := reply["generation"]; exists {
			return reply, errors.New("retired generation exposed")
		}
		if _, exists := reply["projection_phase"]; exists {
			return reply, errors.New("retired projection phase exposed")
		}
		if !add && (reply["removed"] != true || reply["instance_name"] != replayTargetHandle) {
			return reply, errors.New("remove result has wrong handle/outcome")
		}
		return reply, nil
	}) {
		p.h.t.FailNow()
	}
}

// correctionDesired compares the entire retained envelope except the explicitly
// changed Enabled field. Canonical JSON ignores formatting, not fields or values.
func (p *replayQualification) correctionDesired(name string, enabled bool, expected string) string {
	var canonical string
	if !p.h.check(name, 5*time.Second, func() (any, error) {
		kv, err := p.h.js.KeyValue(p.h.ctx, "semstreams_config_setup03a_semsource")
		if err != nil {
			return nil, err
		}
		entry, err := kv.Get(p.h.ctx, "components."+replayTargetHandle)
		if err != nil {
			return nil, err
		}
		var envelope map[string]any
		decoder := json.NewDecoder(bytes.NewReader(entry.Value()))
		decoder.UseNumber()
		if err := decoder.Decode(&envelope); err != nil {
			return nil, err
		}
		config, _ := envelope["config"].(map[string]any)
		if envelope["name"] != "doc-source" || envelope["type"] != "processor" || envelope["enabled"] != enabled || len(config) == 0 {
			return envelope, errors.New("complete retained desired envelope mismatch")
		}
		detail := map[string]any{"kv_revision": entry.Revision(), "envelope": json.RawMessage(entry.Value())}
		delete(envelope, "enabled")
		raw, err := json.Marshal(envelope)
		if err != nil {
			return detail, err
		}
		canonical = string(raw)
		if expected != "" && canonical != expected {
			return detail, errors.New("retained source envelope changed beyond Enabled")
		}
		return detail, nil
	}) {
		p.h.t.FailNow()
	}
	return canonical
}

func (p *replayQualification) correctionRepeatedRemove() {
	raw, err := p.h.rpc("graph.ingest.remove.setup03a", map[string]any{"instance_name": replayTargetHandle})
	p.h.check("repeated_successful_remove_is_not_found", time.Nanosecond, func() (any, error) {
		var reply struct {
			Removed bool                  `json:"removed"`
			Error   struct{ Code string } `json:"error"`
		}
		if decodeErr := json.Unmarshal(raw, &reply); decodeErr != nil {
			return string(raw), decodeErr
		}
		if err == nil || reply.Error.Code != "NOT_FOUND" || reply.Removed {
			return reply, errors.New("repeated desired remove falsely claimed success")
		}
		return json.RawMessage(raw), nil
	})
}

func (p *replayQualification) correctionUnavailable(stage string) {
	before := map[string]uint64{}
	for _, id := range p.targetIDs {
		entity, err := p.h.entity(id)
		if err != nil {
			p.h.t.Fatal(err)
		}
		before[id] = entity.Revision
	}
	for _, subject := range []string{"graph.lifecycle.source", "graph.lifecycle.run"} {
		input, err := json.Marshal(map[string]any{"org": "setup03a", "systems": []string{replayTargetProject}, "reason": "source_removed"})
		if err != nil {
			p.h.t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(p.h.ctx, 5*time.Second)
		reply, err := p.h.nc.RequestWithContext(ctx, subject, input)
		cancel()
		p.h.check(stage+"_"+subject+"_unavailable", time.Nanosecond, func() (any, error) {
			if err != nil {
				return nil, err
			}
			var body struct {
				Message string                `json:"message"`
				Error   struct{ Code string } `json:"error"`
			}
			if err := json.Unmarshal(reply.Data, &body); err != nil {
				return string(reply.Data), err
			}
			detail := map[string]any{"headers": reply.Header, "body": json.RawMessage(reply.Data)}
			if subject == "graph.lifecycle.source" {
				if body.Error.Code != "SOURCE_LIFECYCLE_UNAVAILABLE" {
					return detail, errors.New("expected explicit retired projection refusal")
				}
			} else if reply.Header.Get("X-Status") != "error" || body.Message != "SOURCE_LIFECYCLE_UNAVAILABLE: source lifecycle projection is unavailable" {
				return detail, errors.New("expected header-classified legacy projection refusal")
			}
			return detail, nil
		})
	}
	p.h.check(stage+"_lifecycle_http_gone", 3*time.Second, func() (any, error) {
		ctx, cancel := context.WithTimeout(p.h.ctx, 2*time.Second)
		defer cancel()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.h.httpURL+"/source-manifest/sources/"+replayTargetHandle+"/lifecycle", nil)
		if err != nil {
			return nil, err
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(response.Body, 4096))
		if err != nil {
			return nil, err
		}
		var body struct {
			Error *struct{ Code string } `json:"error"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return string(raw), err
		}
		if response.StatusCode != http.StatusGone || body.Error == nil || body.Error.Code != "SOURCE_LIFECYCLE_UNAVAILABLE" {
			return string(raw), fmt.Errorf("lifecycle status=%d, want 410/unavailable", response.StatusCode)
		}
		return map[string]any{"status": response.StatusCode, "body": json.RawMessage(raw)}, nil
	})
	p.h.check(stage+"_refusals_preserve_target_revisions", time.Nanosecond, func() (any, error) {
		for id, revision := range before {
			entity, err := p.h.entity(id)
			if err != nil {
				return entity, err
			}
			if entity.Revision != revision {
				return entity, fmt.Errorf("refused projection changed revision of %s", id)
			}
		}
		return before, nil
	})
}
