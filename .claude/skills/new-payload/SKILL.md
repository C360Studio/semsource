---
name: new-payload
description: Step-by-step checklist for adding a new payload type to the registry. Use when creating new message types for the agentic system or any polymorphic message flow.
argument-hint: [PayloadTypeName]
---

# New Payload Type Checklist

## What payload type are you adding?

$ARGUMENTS

## Step 1: Define the Type

Create your message struct with JSON tags and domain constants:

```go
// yourpackage/types.go
const (
    Domain          = "yourdomain"
    CategoryYourCat = "your_category"
    Version         = "v1"
)

type YourMessage struct {
    ID      string `json:"id"`
    Content string `json:"content"`
    // ... your fields
}
```

## Step 2: Implement MarshalJSON

**MUST wrap in BaseMessage. MUST use type alias to avoid infinite recursion.**

```go
// yourpackage/types.go
func (m *YourMessage) MarshalJSON() ([]byte, error) {
    type Alias YourMessage
    return json.Marshal(&message.BaseMessage{
        Type: message.MessageType{
            Domain:   Domain,
            Category: CategoryYourCat,
            Version:  Version,
        },
        Payload: (*Alias)(m),
    })
}
```

## Step 3: Register in init()

Create a `payload_registry.go` file in your package:

```go
// yourpackage/payload_registry.go
package yourpackage

import "github.com/c360studio/semstreams/component"

func init() {
    err := component.RegisterPayload(&component.PayloadRegistration{
        Domain:      Domain,
        Category:    CategoryYourCat,
        Version:     Version,
        Description: "Description of your message type",
        Factory:     func() any { return &YourMessage{} },
    })
    if err != nil {
        panic("failed to register YourMessage: " + err.Error())
    }
}
```

## Step 4: Import the Package

Ensure the package is imported somewhere so `init()` runs:

```go
import _ "github.com/c360studio/semstreams/yourpackage"
```

Check existing blank imports in `cmd/` or `service/` entry points for the pattern.

## Step 5: Write Round-Trip Test

```go
func TestYourMessage_RoundTrip(t *testing.T) {
    original := &YourMessage{ID: "test-1", Content: "hello"}

    data, err := json.Marshal(original)
    require.NoError(t, err)

    // Verify JSON has type wrapper
    require.Contains(t, string(data), `"domain":"yourdomain"`)

    var base message.BaseMessage
    err = json.Unmarshal(data, &base)
    require.NoError(t, err)

    result, ok := base.Payload.(*YourMessage)
    require.True(t, ok, "expected *YourMessage, got %T", base.Payload)
    assert.Equal(t, original.ID, result.ID)
    assert.Equal(t, original.Content, result.Content)
}
```

## Verification Checklist

- [ ] Domain/Category/Version constants match between registration and MarshalJSON
- [ ] MarshalJSON uses type alias (`type Alias YourMessage`) to prevent recursion
- [ ] `payload_registry.go` exists with `init()` function
- [ ] Package is imported (blank import if needed) in an entry point
- [ ] Round-trip test passes: `go test -run TestYourMessage_RoundTrip ./yourpackage/...`
- [ ] `task schema:generate` produces no diff (commit schemas if changed)

## Common Mistakes

| Symptom | Cause | Fix |
|---------|-------|-----|
| JSON missing `"type"` wrapper | Missing MarshalJSON | Implement MarshalJSON wrapping in BaseMessage |
| Deserializes as `*message.GenericPayload` | Domain/Category/Version mismatch | Ensure constants match between registration and MarshalJSON |
| Payload never appears in registry | Package not imported | Add blank import in entry point |
| Stack overflow on Marshal | No type alias in MarshalJSON | Add `type Alias YourMessage` before marshal call |
| Schema validation fails in CI | Forgot `task schema:generate` | Run it and commit the generated files |

## Debugging

```go
// List all registered payloads
payloads := component.GlobalPayloadRegistry().ListPayloads()
for msgType, reg := range payloads {
    fmt.Printf("%s: %s\n", msgType, reg.Description)
}

// Verify JSON structure
data, _ := json.MarshalIndent(msg, "", "  ")
fmt.Println(string(data))
// Should show: {"type":{"domain":"...","category":"...","version":"..."},"payload":{...}}
```

Read `docs/concepts/15-payload-registry.md` for full documentation.
