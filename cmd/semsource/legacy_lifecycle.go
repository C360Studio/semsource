package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	semconfig "github.com/c360studio/semengine/config"
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

// legacySemStreamsConfigBucket returns the pre-SemEngine config KV bucket name
// ("semstreams_config_<org>_<stem>") for the given platform identity.
func legacySemStreamsConfigBucket(org, stem string) (string, error) {
	bucket, err := semconfig.BucketName(org, stem)
	if err != nil {
		return "", err
	}
	return strings.Replace(bucket, "semengine_config", "semstreams_config", 1), nil
}

// checkLegacySemStreamsConfig refuses to boot against a JetStream store that
// still retains a populated SemStreams config bucket ("semstreams_config_<org>_<stem>").
// SemEngine renames the config bucket to "semengine_config_<org>_<stem>", so an
// in-place upgrade would silently orphan runtime-added source components while
// leaving their entities in ENTITY_STATES.
func checkLegacySemStreamsConfig(ctx context.Context, reader legacyBucketReader, org, stem string) error {
	currentBucket, err := semconfig.BucketName(org, stem)
	if err != nil {
		return err
	}
	legacyBucket := strings.Replace(currentBucket, "semengine_config", "semstreams_config", 1)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	bucket, err := reader.GetKeyValueBucket(ctx, legacyBucket)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect incompatible legacy SemStreams config bucket %s: %w", legacyBucket, err)
	}
	status, err := bucket.Status(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("inspect incompatible legacy SemStreams config bucket %s: %w", legacyBucket, err)
	}
	if status.Values() != 0 {
		return fmt.Errorf(
			"incompatible legacy SemStreams config bucket %s contains retained state (%d entries); "+
				"SemEngine uses %s and requires a clean JetStream storage boundary "+
				"(stop former SemStreams writers, provision a fresh JetStream store or purge legacy SemStreams state, and reseed from source)",
			legacyBucket, status.Values(), currentBucket,
		)
	}
	return nil
}
