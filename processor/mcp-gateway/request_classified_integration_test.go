//go:build integration

package mcpgateway

import (
	"context"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
)

// TestIntegration_RequestSurfacesClassifiedHandlerErrors pins the audit's
// "errors masquerade as answers" fix: an ADR-060 classified error reply
// (X-Error-Class/X-Error-Code headers + {message, detail} body) from a
// downstream handler must surface as a Go error from the gateway's request
// seam — and therefore as an MCP isError result — never as successful tool
// text. With the old plain Request() the envelope body came back as a normal
// answer, detectable only by the missing contract_version.
func TestIntegration_RequestSurfacesClassifiedHandlerErrors(t *testing.T) {
	tc := natsclient.NewTestClient(t)
	ctx := context.Background()

	// A downstream handler that fails: SubscribeForRequests replies with the
	// framework's classified error envelope automatically.
	const subject = "test.mcpgw.classified"
	sub, err := tc.Client.SubscribeForRequests(ctx, subject,
		func(context.Context, []byte) ([]byte, error) {
			return nil, errs.WrapInvalid(errs.ErrInvalidData, "code-context", "serve", "index rebuilding")
		})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	c := &Component{client: tc.Client}
	resp, reqErr := c.request(ctx, subject, nil)
	if reqErr == nil {
		t.Fatalf("request returned success for a classified error reply; body = %s", resp)
	}
	if !strings.Contains(reqErr.Error(), "index rebuilding") {
		t.Errorf("error lost the handler message: %v", reqErr)
	}

	// Control: a successful reply passes through unchanged.
	const okSubject = "test.mcpgw.ok"
	okSub, err := tc.Client.SubscribeForRequests(ctx, okSubject,
		func(context.Context, []byte) ([]byte, error) {
			return []byte(`{"contract_version":"1","truncated":false}`), nil
		})
	if err != nil {
		t.Fatalf("subscribe ok: %v", err)
	}
	t.Cleanup(func() { _ = okSub.Unsubscribe() })

	body, okErr := c.request(ctx, okSubject, nil)
	if okErr != nil {
		t.Fatalf("request failed for a successful reply: %v", okErr)
	}
	if !strings.Contains(string(body), "contract_version") {
		t.Errorf("success body mangled: %s", body)
	}
}
