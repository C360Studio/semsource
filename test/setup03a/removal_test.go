//go:build qualification && removal

package setup03a

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// TestSourceRemovalLifecycle is a separate supplemental qualification. Select it
// with -tags=qualification,removal so the frozen paired corpus retains its counts.
// SETUP03A_REMOVAL_ACTIVATION selects the public contract: runtime for beta.161,
// restart for the pinned boot-only composition. Neither mode repairs the graph.
func TestSourceRemovalLifecycle(t *testing.T) {
	binary, out := os.Getenv("SETUP03A_BINARY"), os.Getenv("SETUP03A_OUT")
	activation := os.Getenv("SETUP03A_REMOVAL_ACTIVATION")
	if binary == "" || out == "" || (activation != "runtime" && activation != "restart") {
		t.Fatal("SETUP03A_BINARY, SETUP03A_OUT and SETUP03A_REMOVAL_ACTIVATION=runtime|restart are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	h := &harness{t: t, ctx: ctx, out: out, binary: binary}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	h.result = report{CorpusVersion: "1", Profile: "bm25-source-removal", SemEngineSlices: []int{0, 1}, Started: time.Now().UTC()}
	defer h.finish()
	binaryBytes, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(binaryBytes)
	h.result.BinarySHA256 = hex.EncodeToString(sum[:])
	h.fixture()
	h.broker()
	h.configure("bm25")
	h.start()
	var instance string
	if !h.check("removal_initial_settled_sources", 45*time.Second, func() (any, error) {
		status, raw, statusErr := h.removalStatus()
		if statusErr != nil {
			return raw, statusErr
		}
		if status.Phase != "ready" || len(status.Sources) != 2 {
			return raw, errors.New("expected both admitted sources ready")
		}
		for _, source := range status.Sources {
			if source.Offered != source.Delivered || source.Lost != 0 || source.SeedLost != 0 {
				return raw, errors.New("source delivery is unsettled or lost")
			}
			if source.Type == "docs" {
				instance = source.Instance
			}
		}
		if instance == "" {
			return raw, errors.New("document source handle absent")
		}
		return raw, nil
	}) {
		return
	}
	h.resolveIdentity()
	prefix := "setup03a.semsource.web.setup03a-docs"
	if os.Getenv("SETUP03A_IDENTITY") == "governed" {
		prefix = "setup03a." + strings.Split(h.ids["run"], ".")[1] + ".setup03a-docs.web"
	}
	var documentIDs []string
	if !h.check("removal_initial_document_entities", 20*time.Second, func() (any, error) {
		raw, queryErr := h.rpc("graph.query.prefix", map[string]any{"prefix": prefix, "limit": 10})
		if queryErr != nil {
			return json.RawMessage(raw), queryErr
		}
		var response struct {
			Entities []struct {
				ID string `json:"id"`
			} `json:"entities"`
		}
		if err := json.Unmarshal(raw, &response); err != nil {
			return json.RawMessage(raw), err
		}
		ids, kinds := []string{}, map[string]bool{}
		for _, entity := range response.Entities {
			parts := strings.Split(entity.ID, ".")
			if len(parts) != 6 {
				return json.RawMessage(raw), fmt.Errorf("invalid document ID %q", entity.ID)
			}
			ids = append(ids, entity.ID)
			kinds[parts[4]] = true
			e, entityErr := h.entity(entity.ID)
			if entityErr != nil || value(e, "entity.lifecycle.stale") != nil {
				return e, fmt.Errorf("initial entity must exist and be current: %v", entityErr)
			}
		}
		if len(ids) != 2 || !kinds["doc"] || !kinds["chunk"] {
			return json.RawMessage(raw), errors.New("expected one document parent and one passage")
		}
		documentIDs = ids
		return json.RawMessage(raw), nil
	}) {
		return
	}
	// Capture the source's automatic request only. Sending our own lifecycle
	// trigger here would conceal a missing deferred-removal replay.
	sub, err := h.nc.SubscribeSync("graph.lifecycle.run")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.nc.FlushTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	defer func() {
		requests := []json.RawMessage{}
		for {
			msg, readErr := sub.NextMsg(10 * time.Millisecond)
			if errors.Is(readErr, nats.ErrTimeout) {
				break
			}
			if readErr != nil {
				t.Errorf("capture lifecycle requests: %v", readErr)
				break
			}
			requests = append(requests, append(json.RawMessage(nil), msg.Data...))
		}
		raw, marshalErr := json.MarshalIndent(requests, "", "  ")
		if marshalErr != nil {
			t.Error(marshalErr)
		} else {
			h.save("automatic-lifecycle-requests.json", raw)
		}
		if err := sub.Unsubscribe(); err != nil {
			t.Error(err)
		}
	}()
	receipt, removeErr := h.rpc("graph.ingest.remove.setup03a", map[string]any{"instance_name": instance, "provenance": map[string]string{"actor": "setup03a-removal-qualification"}})
	h.save("remove-receipt.json", receipt)
	if !h.check("removal_receipt", time.Second, func() (any, error) {
		if removeErr != nil {
			return json.RawMessage(receipt), removeErr
		}
		var reply struct {
			Removed, DesiredChanged, RuntimeChanged, RestartRequired bool
		}
		var wire map[string]any
		if err := json.Unmarshal(receipt, &wire); err != nil {
			return json.RawMessage(receipt), err
		}
		reply.Removed, _ = wire["removed"].(bool)
		reply.DesiredChanged, _ = wire["desired_changed"].(bool)
		reply.RuntimeChanged, _ = wire["runtime_changed"].(bool)
		reply.RestartRequired, _ = wire["restart_required"].(bool)
		if wire["error"] != nil || !reply.Removed || (activation == "restart" && (!reply.DesiredChanged || reply.RuntimeChanged || !reply.RestartRequired)) {
			return json.RawMessage(receipt), errors.New("removal receipt violates selected activation contract")
		}
		return json.RawMessage(receipt), nil
	}) {
		return
	}
	if activation == "restart" {
		h.check("removal_deferred_until_restart", time.Second, func() (any, error) {
			status, raw, statusErr := h.removalStatus()
			if statusErr != nil {
				return raw, statusErr
			}
			for _, source := range status.Sources {
				if source.Instance == instance {
					return raw, nil
				}
			}
			return raw, errors.New("desired removal changed the admitted runtime")
		})
		h.stop(false)
		h.start()
	}
	h.check("removal_activated", 15*time.Second, func() (any, error) {
		status, raw, statusErr := h.removalStatus()
		if statusErr != nil {
			return raw, statusErr
		}
		for _, source := range status.Sources {
			if source.Instance == instance {
				return raw, errors.New("removed source remains admitted")
			}
		}
		if status.Phase != "ready" || len(status.Sources) != 1 {
			return raw, errors.New("remaining source is not ready")
		}
		return raw, nil
	})
	h.check("removed_parent_and_passage_retained_with_source_removed", 20*time.Second, func() (any, error) {
		entities := make(map[string]exact, len(documentIDs))
		var failures []error
		for _, id := range documentIDs {
			e, entityErr := h.entity(id)
			entities[id] = e
			if entityErr != nil {
				failures = append(failures, entityErr)
			} else if marker := value(e, "entity.lifecycle.stale"); marker != "source_removed" {
				failures = append(failures, fmt.Errorf("retained entity %s marker=%v, want source_removed", id, marker))
			}
		}
		return entities, errors.Join(failures...)
	})
	if activation == "restart" {
		h.removalReadd(instance)
	}
}

type removalStatus struct {
	Phase   string `json:"phase"`
	Sources []struct {
		Instance  string `json:"instance_name"`
		Type      string `json:"source_type"`
		Offered   int64  `json:"offered_total"`
		Delivered int64  `json:"delivered_total"`
		Lost      int64  `json:"lost_total"`
		SeedLost  int64  `json:"seed_lost"`
	} `json:"sources"`
}

func (h *harness) removalStatus() (removalStatus, json.RawMessage, error) {
	raw, err := h.rpc("graph.query.status", map[string]any{})
	var status removalStatus
	if err == nil {
		err = json.Unmarshal(raw, &status)
	}
	return status, json.RawMessage(raw), err
}

func (h *harness) removalReadd(instance string) {
	receipt, addErr := h.rpc("graph.ingest.add.setup03a", map[string]any{
		"source":     map[string]any{"type": "docs", "paths": []string{filepath.Join(h.source, "docs")}, "project": "setup03a-docs", "watch": true},
		"provenance": map[string]string{"actor": "setup03a-removal-qualification"},
	})
	h.save("readd-receipt.json", receipt)
	if !h.check("readd_receipt_same_disabled_handle", time.Second, func() (any, error) {
		var reply struct {
			Error           any  `json:"error"`
			DesiredChanged  bool `json:"desired_changed"`
			RuntimeChanged  bool `json:"runtime_changed"`
			RestartRequired bool `json:"restart_required"`
			Components      []struct {
				Instance string `json:"instance_name"`
				Created  bool   `json:"created"`
			} `json:"components"`
		}
		if addErr != nil {
			return json.RawMessage(receipt), addErr
		}
		if err := json.Unmarshal(receipt, &reply); err != nil {
			return json.RawMessage(receipt), err
		}
		if reply.Error != nil || !reply.DesiredChanged || reply.RuntimeChanged || !reply.RestartRequired || len(reply.Components) != 1 || reply.Components[0].Instance != instance || !reply.Components[0].Created {
			return json.RawMessage(receipt), errors.New("re-add did not restore the same disabled desired handle")
		}
		return json.RawMessage(receipt), nil
	}) {
		return
	}
	h.check("readd_deferred_until_restart", time.Second, func() (any, error) {
		status, raw, err := h.removalStatus()
		if err != nil {
			return raw, err
		}
		if len(status.Sources) != 1 || status.Sources[0].Instance == instance {
			return raw, errors.New("re-add changed admitted runtime")
		}
		return raw, nil
	})
	h.stop(false)
	h.start()
	h.check("readd_activated_after_restart", 15*time.Second, func() (any, error) {
		status, raw, err := h.removalStatus()
		if err != nil {
			return raw, err
		}
		if status.Phase != "ready" || len(status.Sources) != 2 {
			return raw, errors.New("re-added source not ready")
		}
		for _, source := range status.Sources {
			if source.Instance == instance {
				return raw, nil
			}
		}
		return raw, errors.New("same source handle missing after re-add restart")
	})
}
