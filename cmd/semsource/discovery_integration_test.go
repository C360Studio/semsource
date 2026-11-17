//go:build integration

package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semengine/service"

	"github.com/c360studio/semsource/internal/natstest"
)

// TestIntegrationDiscoveryBootSeam proves the seam boot-time discovery uses
// (design D1): a component put into the config manager after arbitration and
// before the service manager is built is in the snapshot the component
// manager constructs from (semengine service/component_manager.go:188), on a
// retained store at an equal version where the file itself would not win.
//
// spec: openspec/changes/boot-time-source-discovery/specs/source-discovery/spec.md#Requirement: Discovery takes effect at the boot that finds it
func TestIntegrationDiscoveryBootSeam(t *testing.T) {
	broker := natstest.New(t, natstest.WithJetStream())
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "semsource.json")
	writeFile(t, path, revisionFile("", guideSource))
	bootConfigFile(ctx, t, broker.URL, path) // first boot writes the file to the bucket

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg, _, err := loadAndExpandConfig(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	nc, ssCfg, err := setupNATS(ctx, broker.URL, cfg, logger, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = nc.Close(ctx) }()
	configMgr, err := startConfigManager(ctx, ssCfg, nc, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configMgr.Stop(ctx) }()

	added := configMgr.GetConfig().Get().Components["doc-source-guide"]
	if err := configMgr.PutComponentToKV(ctx, "doc-source-discovered", added); err != nil {
		t.Fatal(err)
	}
	registry, err := prepareComponentRegistry(ctx, cfg, configMgr, nc)
	if err != nil {
		t.Fatal(err)
	}
	payloadReg, err := buildPayloadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	manager, err := createServiceManager(nc, registry, payloadReg, configMgr, logger)
	if err != nil {
		t.Fatal(err)
	}
	svc, ok := manager.GetService("component-manager")
	if !ok {
		t.Fatal("no component-manager service")
	}
	cm, ok := svc.(*service.ComponentManager)
	if !ok {
		t.Fatalf("component-manager is %T", svc)
	}
	if _, ok := cm.GetComponentStatus()["doc-source-discovered"]; !ok {
		t.Fatal("a component put before the service manager is built was not created for this boot")
	}
}
