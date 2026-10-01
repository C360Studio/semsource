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
