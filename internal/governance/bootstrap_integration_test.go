//go:build integration

package governance

import (
	"context"
	"testing"

	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/ownership"
)

func TestIntegration_BootstrapStandalone_BindsSourceOwnership(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	boot, err := BootstrapStandalone(ctx, tc.Client, nil)
	if err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}
	if boot == nil || boot.Registry == nil || boot.Heartbeater == nil {
		t.Fatalf("BootstrapStandalone() returned incomplete bootstrap: %#v", boot)
	}

	reader, err := ownership.NewClaimReader(ctx, tc.Client, nil)
	if err != nil {
		t.Fatalf("NewClaimReader() error = %v", err)
	}

	owner, incarnation, ok, err := reader.OwnerOf(
		ctx,
		"acme.semsource.web.docs.doc.abc123",
		source.DocContent,
	)
	if err != nil {
		t.Fatalf("OwnerOf() error = %v", err)
	}
	if !ok {
		t.Fatal("OwnerOf() ok = false, want true for SemSource-owned doc content")
	}
	if owner != OwnerID {
		t.Fatalf("OwnerOf() owner = %q, want %q", owner, OwnerID)
	}
	if incarnation == "" {
		t.Fatal("OwnerOf() incarnation is empty")
	}
}
