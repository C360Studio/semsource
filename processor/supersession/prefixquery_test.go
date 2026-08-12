package supersession

import (
	"errors"
	"fmt"
	"testing"

	"github.com/c360studio/semstreams/pkg/errs"
)

// response_too_large is a RESULT-SIZE failure: the responder classified the
// encoded result as exceeding the connected server's max payload. Treating it
// as an availability timeout (retry, wait, escalate readiness) would mask a
// contract problem as latency — the beta.160 migration guide calls this out
// explicitly, and this pin keeps the two failure classes separate.
func TestIsResponseTooLarge(t *testing.T) {
	tooLarge := &errs.ClassifiedError{
		Class:   errs.ErrorInvalid,
		Code:    "response_too_large",
		Message: "encoded result exceeds max payload",
	}
	cases := map[string]struct {
		err  error
		want bool
	}{
		"classified response_too_large":         {tooLarge, true},
		"wrapped classified response_too_large": {fmt.Errorf("prefix query: %w", tooLarge), true},
		"other classified code": {&errs.ClassifiedError{
			Class: errs.ErrorInvalid, Code: "entity_not_found",
		}, false},
		"uncoded classified error": {&errs.ClassifiedError{Class: errs.ErrorTransient}, false},
		"plain transport error":    {errors.New("nats: timeout"), false},
		"nil":                      {nil, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isResponseTooLarge(tc.err); got != tc.want {
				t.Fatalf("isResponseTooLarge(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
