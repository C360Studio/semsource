package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/message"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
)

// TestPublishedEnvelopesDecodeThroughTheBootstrapRegistry: source-manifest
// publishes its manifest, predicate schema, and status broadcasts on GRAPH as
// typed envelopes. Anything that decodes them with the payload registry the
// service is built with, such as SemEngine's message logger, needs their
// types registered; without sourcemanifest.RegisterPayloads in
// buildPayloadRegistry the decode fails, and nothing else noticed (#270).
//
// spec: openspec/specs/advertised-surface-coverage/spec.md#Requirement: Published envelopes decode through the bootstrap registry
func TestPublishedEnvelopesDecodeThroughTheBootstrapRegistry(t *testing.T) {
	reg, err := buildPayloadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	decoder := message.NewDecoder(reg)

	published := []struct {
		msgType message.Type
		payload message.Payload
	}{
		{sourcemanifest.ManifestType, &sourcemanifest.ManifestPayload{Namespace: "acme"}},
		{sourcemanifest.PredicatesType, &sourcemanifest.PredicateSchemaPayload{}},
		{sourcemanifest.StatusType, &sourcemanifest.StatusPayload{Namespace: "acme", Phase: "ready"}},
	}
	for _, p := range published {
		t.Run(p.msgType.String(), func(t *testing.T) {
			data, err := json.Marshal(message.NewBaseMessage(p.msgType, p.payload, "semsource"))
			if err != nil {
				t.Fatal(err)
			}
			msg, err := decoder.Decode(data)
			if err != nil {
				t.Fatalf("decode %s envelope: %v", p.msgType, err)
			}
			if got, want := reflect.TypeOf(msg.Payload()), reflect.TypeOf(p.payload); got != want {
				t.Fatalf("decoded payload type = %v, want %v", got, want)
			}
		})
	}
}
