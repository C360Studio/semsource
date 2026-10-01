package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const legacyLifecycleBucket = "SEMSOURCE_SOURCE_LIFECYCLE"

type legacyBucketReader interface {
	GetKeyValueBucket(context.Context, string) (jetstream.KeyValue, error)
}

// checkLegacyLifecycle only admits storage that has no retained private recovery
// state. The former writer must be stopped; this is not an outcome resolver.
func checkLegacyLifecycle(ctx context.Context, reader legacyBucketReader) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	bucket, err := reader.GetKeyValueBucket(ctx, legacyLifecycleBucket)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect incompatible legacy storage %s: %w", legacyLifecycleBucket, err)
	}
	status, err := bucket.Status(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("inspect incompatible legacy storage %s: %w", legacyLifecycleBucket, err)
	}
	if status.Values() != 0 {
		return fmt.Errorf("incompatible legacy storage %s contains retained messages; preserve existing storage and stop its writers; this build requires a separate fresh deployment storage boundary", legacyLifecycleBucket)
	}
	return nil
}
