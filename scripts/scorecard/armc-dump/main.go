// Command armc-dump is the scorecard's read-only NATS access for arm C
// (scripts/scorecard/arm-c-cosine.sh). It exists so the harness can reach the
// product's own stored vectors and bodies from the host in one process,
// instead of thousands of CLI round-trips through a helper container.
//
// Read-only by construction: it opens KV buckets and the object store only to
// get; nothing here writes, deletes, or subscribes to live updates.
//
//	go run ./scripts/scorecard/armc-dump -nats nats://127.0.0.1:24222 vectors
//	    -> one line per EMBEDDING_INDEX entry: "<entity-id>\tv1,v2,..."
//
//	go run ./scripts/scorecard/armc-dump -nats ... bodies <id> [<id>...]
//	    -> NDJSON per id: {"id","key","bytes","body"} with body base64-encoded,
//	       resolved via ENTITY_STATES storage_ref into the CONTENT object store
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func main() {
	natsURL := flag.String("nats", "nats://127.0.0.1:24222", "NATS server URL")
	flag.Parse()
	if flag.NArg() < 1 {
		fatalf("usage: armc-dump [-nats url] vectors | bodies <entity-id>...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	nc, err := nats.Connect(*natsURL)
	if err != nil {
		fatalf("connect %s: %v", *natsURL, err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		fatalf("jetstream: %v", err)
	}

	switch flag.Arg(0) {
	case "vectors":
		err = dumpVectors(ctx, js)
	case "bodies":
		err = dumpBodies(ctx, js, flag.Args()[1:])
	default:
		fatalf("unknown subcommand %q", flag.Arg(0))
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "armc-dump: "+format+"\n", args...)
	os.Exit(1)
}

// dumpVectors streams every EMBEDDING_INDEX entry as "<id>\tv1,v2,...".
// Output order follows the key lister; the consumer ranks by cosine, so
// order carries no meaning.
func dumpVectors(ctx context.Context, js jetstream.JetStream) error {
	kv, err := js.KeyValue(ctx, "EMBEDDING_INDEX")
	if err != nil {
		return fmt.Errorf("open EMBEDDING_INDEX: %w", err)
	}
	lister, err := kv.ListKeys(ctx)
	if err != nil {
		return fmt.Errorf("list EMBEDDING_INDEX keys: %w", err)
	}
	for key := range lister.Keys() {
		entry, err := kv.Get(ctx, key)
		if err != nil {
			return fmt.Errorf("get %s: %w", key, err)
		}
		var rec struct {
			EntityID string    `json:"entity_id"`
			Vector   []float64 `json:"vector"`
		}
		if err := json.Unmarshal(entry.Value(), &rec); err != nil {
			return fmt.Errorf("parse %s: %w", key, err)
		}
		parts := make([]string, len(rec.Vector))
		for i, f := range rec.Vector {
			parts[i] = strconv.FormatFloat(f, 'g', -1, 64)
		}
		fmt.Printf("%s\t%s\n", rec.EntityID, strings.Join(parts, ","))
	}
	return nil
}

// dumpBodies resolves each entity's storage_ref through ENTITY_STATES and
// fetches the body from the CONTENT object store, emitting one NDJSON record
// per id in argument order (deterministic for the caller).
func dumpBodies(ctx context.Context, js jetstream.JetStream, ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("bodies: no entity ids given")
	}
	kv, err := js.KeyValue(ctx, "ENTITY_STATES")
	if err != nil {
		return fmt.Errorf("open ENTITY_STATES: %w", err)
	}
	store, err := js.ObjectStore(ctx, "CONTENT")
	if err != nil {
		return fmt.Errorf("open CONTENT: %w", err)
	}
	enc := json.NewEncoder(os.Stdout)
	for _, id := range ids {
		rec, err := bodyFor(ctx, kv, store, id)
		if err != nil {
			return err
		}
		if err := enc.Encode(rec); err != nil {
			return fmt.Errorf("encode %s: %w", id, err)
		}
	}
	return nil
}

type bodyRecord struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Bytes int    `json:"bytes"`
	Body  string `json:"body"`
}

func bodyFor(ctx context.Context, kv jetstream.KeyValue, store jetstream.ObjectStore, id string) (bodyRecord, error) {
	entry, err := kv.Get(ctx, id)
	if err != nil {
		return bodyRecord{}, fmt.Errorf("ENTITY_STATES get %s: %w", id, err)
	}
	var state struct {
		StorageRef struct {
			Key string `json:"key"`
		} `json:"storage_ref"`
	}
	if err := json.Unmarshal(entry.Value(), &state); err != nil {
		return bodyRecord{}, fmt.Errorf("parse state %s: %w", id, err)
	}
	// Entities without an offloaded body (no storage_ref) have nothing to
	// grade against; report them explicitly rather than failing the batch.
	if state.StorageRef.Key == "" {
		return bodyRecord{ID: id}, nil
	}
	data, err := store.GetBytes(ctx, state.StorageRef.Key)
	if err != nil {
		return bodyRecord{}, fmt.Errorf("CONTENT get %s (%s): %w", state.StorageRef.Key, id, err)
	}
	return bodyRecord{
		ID:    id,
		Key:   state.StorageRef.Key,
		Bytes: len(data),
		Body:  base64.StdEncoding.EncodeToString(data),
	}, nil
}
