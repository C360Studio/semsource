---
name: new-payload
description: Step-by-step checklist for adding a new payload type to the registry. Use when creating new message types for the agentic system or any polymorphic message flow.
argument-hint: [PayloadTypeName]
---

> **Product-specific recipe.** Both repositories now register explicitly at bootstrap (ADR-103).
> This fork keeps SemSource's package references and composition root. The pinned registry owns
> payload schema, indexing-profile floors, and projection contracts; no `init()` or blank imports.

# New Payload Type Checklist

Canonical references in this repo: `graph/event_payload.go` (EntityPayload)
and `processor/source-manifest/payload_registry.go` + `payload_status.go`
(multiple payloads in one package). Registration is explicit at bootstrap —
there is **no** `init()` registration and **no** blank-importing.

## What payload type are you adding?

$ARGUMENTS

## Step 1: Define the Type and its message.Type

```go
// yourpackage/payload_yourthing.go

// YourPayloadType is the message type for your payloads.
var YourPayloadType = message.Type{Domain: "semsource", Category: "your_category", Version: "v1"}

type YourPayload struct {
    ID      string `json:"id"`
    Content string `json:"content"`
    // ... your fields
}
```

## Step 2: Implement message.Payload

`Schema()` must return the same Domain/Category/Version the registration
declares. Registration rejects a mismatch; the production decoder rejects an unregistered type.

```go
// Schema implements message.Payload.
func (p *YourPayload) Schema() message.Type { return YourPayloadType }

// Validate implements message.Payload.
func (p *YourPayload) Validate() error {
    if p.ID == "" {
        return errors.New("id is required")
    }
    return nil
}
```

## Step 3: Alias-based Marshal/Unmarshal

**MUST use a type alias to avoid infinite recursion.** No `BaseMessage`
wrapping — the transport envelope is applied by the messaging layer, not by
the payload.

```go
// MarshalJSON implements json.Marshaler.
func (p *YourPayload) MarshalJSON() ([]byte, error) {
    type Alias YourPayload
    return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *YourPayload) UnmarshalJSON(data []byte) error {
    type Alias YourPayload
    return json.Unmarshal(data, (*Alias)(p))
}
```

## Step 4: Add to the package's RegisterPayloads

Each payload-owning package exposes one `RegisterPayloads`; add a
`Registration` to it (create the file if the package has none):

```go
// yourpackage/payload_registry.go
func RegisterPayloads(reg *payloadregistry.Registry) error {
    return errors.Join(
        // ...existing registrations...
        reg.Register(&payloadregistry.Registration{
            Domain:      "semsource",
            Category:    "your_category",
            Version:     "v1",
            Description: "One-line description of the message",
            Factory:     func() any { return &YourPayload{} },
            IndexingProfile: vocabulary.IndexingProfileControl,
            // For graph facts, bind the applicable projection contracts here.
            // Contracts: []contract.Contract{YourBirthContract()},
        }),
    )
}
```

## Step 5: Wire into bootstrap

`cmd/semsource/run.go`'s `buildPayloadRegistry()` layers semsource payloads
over the semstreams builtins. An existing package (`graph`,
`sourcemanifest`) is already wired; a **new** package needs its call added:

```go
if err := yourpackage.RegisterPayloads(reg); err != nil {
    return nil, fmt.Errorf("register yourpackage payloads: %w", err)
}
```

The registry reaches every component via `service.Dependencies.PayloadRegistry`.

The `IndexingProfile` floor applies when a producer omits its profile. Select it according to the
payload's semantics; do not default content payloads to control. Bind graph birth/projection contracts
on the registration (ADR-103). Contract `MessageType` is a structured `message.Type`; an empty value
is filled by registration and a conflicting type is rejected. Every `entity.create` type must be
registered or graph-ingest returns `message_type_unregistered`.

## Step 6: Round-Trip Test

```go
func TestYourPayload_RoundTrip(t *testing.T) {
    original := &YourPayload{ID: "test-1", Content: "hello"}

    data, err := json.Marshal(original)
    if err != nil {
        t.Fatal(err)
    }
    var decoded YourPayload
    if err := json.Unmarshal(data, &decoded); err != nil {
        t.Fatal(err)
    }
    if decoded != *original {
        t.Fatalf("round-trip mismatch: %+v != %+v", decoded, *original)
    }
    if err := original.Validate(); err != nil {
        t.Fatalf("valid payload rejected: %v", err)
    }
}
```

Also test the production decoder, not only direct JSON serialization: create a
`payloadregistry.Registry`, call the package's `RegisterPayloads`, wrap the payload with
`message.NewBaseMessage`, marshal it, and decode using `message.NewDecoder(reg).Decode`.
Assert the typed payload, schema, and content. Include applicable floor/contract validation and
unregistered-create rejection; a factory lookup alone cannot prove the wire registration path.

## Verification Checklist

- [ ] `Schema()`'s message.Type matches the `Registration`'s Domain/Category/Version exactly
- [ ] Marshal/Unmarshal use a type alias (`type Alias YourPayload`) to prevent recursion
- [ ] The package's `RegisterPayloads` includes the new `Registration`
- [ ] `buildPayloadRegistry()` in `cmd/semsource/run.go` calls the package's `RegisterPayloads` (new packages only)
- [ ] Production-decoder round-trip, schema/floor/contract validation, and registration tests pass
- [ ] `Validate()` rejects the zero value's missing required fields

## Common Mistakes

| Symptom | Cause | Fix |
|---------|-------|-----|
| Registration schema-consistency error | `Schema()` and registration disagree | Define one `message.Type` and use its matching key |
| Unregistered payload/create error | Missing bootstrap registration | Trace the executable registration call from the relevant binary |
| Payload never resolvable at runtime | Package's `RegisterPayloads` not called at bootstrap | Add the call in `buildPayloadRegistry()` (`cmd/semsource/run.go`) |
| Stack overflow on Marshal | No type alias in MarshalJSON/UnmarshalJSON | Add `type Alias YourPayload` before the call |
| Consumer silently misses new fields | Consumer decodes into its own private copy of the struct | Both sides must import the ONE shared type; never re-declare a wire shape (see #188) |
