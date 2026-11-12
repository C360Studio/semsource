package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type legacyReaderStub struct {
	bucket jetstream.KeyValue
	err    error
	calls  int
}

func (r *legacyReaderStub) GetKeyValueBucket(_ context.Context, name string) (jetstream.KeyValue, error) {
	r.calls++
	if name != legacyLifecycleBucket {
		panic("wrong legacy bucket")
	}
	return r.bucket, r.err
}

type legacyBucketStub struct {
	jetstream.KeyValue
	count uint64
	err   error
}

func (b legacyBucketStub) Status(context.Context) (jetstream.KeyValueStatus, error) {
	return legacyStatusStub{count: b.count}, b.err
}

type legacyStatusStub struct {
	jetstream.KeyValueStatus
	count uint64
}

func (s legacyStatusStub) Values() uint64 { return s.count }

func TestLegacyLifecycleCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name      string
		reader    legacyReaderStub
		wantError bool
	}{
		{"absent", legacyReaderStub{err: fmt.Errorf("wrapped: %w", jetstream.ErrBucketNotFound)}, false},
		{"empty", legacyReaderStub{bucket: legacyBucketStub{}}, false},
		{"nonempty", legacyReaderStub{bucket: legacyBucketStub{count: 1}}, true},
		{"lookup_unreadable", legacyReaderStub{err: errors.New("permission denied")}, true},
		{"status_unreadable", legacyReaderStub{bucket: legacyBucketStub{err: errors.New("status unavailable")}}, true},
		{"status_disappeared", legacyReaderStub{bucket: legacyBucketStub{err: jetstream.ErrBucketNotFound}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkLegacyLifecycle(t.Context(), &tc.reader)
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v", err)
			}
			if tc.reader.calls != 1 {
				t.Fatalf("lookup count=%d", tc.reader.calls)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &legacyReaderStub{err: jetstream.ErrBucketNotFound}
	if err := checkLegacyLifecycle(ctx, reader); !errors.Is(err, context.Canceled) || reader.calls != 0 {
		t.Fatalf("canceled probe=%v calls=%d", err, reader.calls)
	}
	expired, stop := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer stop()
	if err := checkLegacyLifecycle(expired, reader); !errors.Is(err, context.DeadlineExceeded) || reader.calls != 0 {
		t.Fatalf("expired probe=%v calls=%d", err, reader.calls)
	}
}

type legacyNamedReaderStub struct {
	wantName string
	bucket   jetstream.KeyValue
	err      error
	calls    int
}

func (r *legacyNamedReaderStub) GetKeyValueBucket(_ context.Context, name string) (jetstream.KeyValue, error) {
	r.calls++
	if name != r.wantName {
		panic("unexpected bucket lookup: " + name)
	}
	return r.bucket, r.err
}

func TestLegacySemStreamsConfigCompatibility(t *testing.T) {
	const wantBucket = "semstreams_config_acme_semsource"
	got, err := legacySemStreamsConfigBucket("acme", "semsource")
	if err != nil || got != wantBucket {
		t.Fatalf("legacySemStreamsConfigBucket = (%q, %v), want (%q, nil)", got, err, wantBucket)
	}
	for _, tc := range []struct {
		name      string
		reader    legacyNamedReaderStub
		wantError bool
	}{
		{"absent", legacyNamedReaderStub{wantName: wantBucket, err: fmt.Errorf("wrapped: %w", jetstream.ErrBucketNotFound)}, false},
		{"empty", legacyNamedReaderStub{wantName: wantBucket, bucket: legacyBucketStub{}}, false},
		{"nonempty", legacyNamedReaderStub{wantName: wantBucket, bucket: legacyBucketStub{count: 2}}, true},
		{"lookup_unreadable", legacyNamedReaderStub{wantName: wantBucket, err: errors.New("permission denied")}, true},
		{"status_unreadable", legacyNamedReaderStub{wantName: wantBucket, bucket: legacyBucketStub{err: errors.New("status unavailable")}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkLegacySemStreamsConfig(t.Context(), &tc.reader, "acme", "semsource")
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v, wantError=%v", err, tc.wantError)
			}
			if tc.reader.calls != 1 {
				t.Fatalf("lookup count=%d", tc.reader.calls)
			}
		})
	}
}
