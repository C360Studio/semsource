package entitypub

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/c360studio/semsource/graph"
)

// snapshotPayload owns the accepted wire values, including mutable triple
// objects and storage references. It retains no publication or recovery state.
func snapshotPayload(payload *graph.EntityPayload) (*graph.EntityPayload, error) {
	if payload == nil {
		return nil, fmt.Errorf("nil entity payload")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode entity payload: %w", err)
	}
	// Bypass EntityPayload.UnmarshalJSON so interface-valued integers do not
	// pass through float64 and lose precision before transport serialization.
	type snapshot graph.EntityPayload
	var owned snapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&owned); err != nil {
		return nil, fmt.Errorf("decode entity payload snapshot: %w", err)
	}
	return (*graph.EntityPayload)(&owned), nil
}
