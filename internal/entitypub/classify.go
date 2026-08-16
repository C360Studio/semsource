package entitypub

// Publish-error classification (retryable-publish-classification / #176).
// publishOne used to retry only when err.Error() equaled the literal breaker
// string. Measured on the 2026-08-16 OSH acceptance, that made the two most
// ordinary transients terminal: a 120s broker pause permanently failed 5,282
// entities, and a stream-capacity refusal failed 34,871 on first attempt —
// upstream deliberately keeps capacity errors circuit-NEUTRAL (a full stream
// is not a broken transport), so the breaker path can never cover them.

import (
	"context"
	"errors"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// publishErrClass is the retry decision for one publish error.
type publishErrClass int

const (
	// publishRetryable: transient transport trouble — retry within the budget.
	publishRetryable publishErrClass = iota
	// publishTerminal: retrying cannot cure it — fail the entity now, loudly.
	publishTerminal
	// publishCanceled: the run context ended (shutdown drain) — neither a
	// retry nor a failure; the caller's ctx guard owns it.
	publishCanceled
)

// streamCapacityDescriptions mirrors upstream's circuit-neutral capacity set
// (natsclient isCircuitNeutralStreamCapacityError): the 10077 refusals a
// discard:new stream emits at its ceiling.
var streamCapacityDescriptions = map[string]bool{
	"maximum bytes exceeded":                true,
	"maximum messages exceeded":             true,
	"maximum messages per subject exceeded": true,
}

// classifyPublishError decides retryable vs terminal by error CLASS, never by
// string comparison. Unknown errors stay terminal on purpose: a marshal or
// subject bug retried 20 times is noise hiding a code defect, and a
// misclassified transient shows up NAMED in the failure log where it can be
// promoted to the retryable set with evidence.
func classifyPublishError(ctx context.Context, err error) publishErrClass {
	if err == nil {
		return publishRetryable // callers never pass nil; keep the zero sane
	}
	if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, ctx.Err())) {
		return publishCanceled
	}
	if errors.Is(err, natsclient.ErrCircuitOpen) ||
		errors.Is(err, natsclient.ErrNotConnected) ||
		errors.Is(err, nats.ErrTimeout) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, nats.ErrNoResponders) {
		return publishRetryable
	}
	var apiErr *jetstream.APIError
	if errors.As(err, &apiErr) && apiErr != nil &&
		apiErr.ErrorCode == jetstream.ErrorCode(10077) &&
		streamCapacityDescriptions[apiErr.Description] {
		return publishRetryable
	}
	return publishTerminal
}
