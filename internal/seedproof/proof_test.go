package seedproof

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestProofIsScopedAndConcurrent(t *testing.T) {
	plain := context.Background()
	if Active(plain) {
		t.Fatal("ordinary ingestion has proof scope")
	}
	Report(plain, errors.New("ordinary skip"))
	ctx, proof := Begin(plain)
	if !Active(ctx) || proof.Err() != nil {
		t.Fatal("new proof not empty")
	}
	Report(ctx, nil)
	want := errors.New("body write failed")
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); Report(ctx, want) }()
	}
	workers.Wait()
	if !errors.Is(proof.Err(), want) {
		t.Fatal("concurrent failure omitted")
	}
	_, separate := Begin(plain)
	if separate.Err() != nil {
		t.Fatal("proof leaked into a later seed")
	}
}
