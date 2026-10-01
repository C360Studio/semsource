package supersession

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semsource/graph"
)

func TestRetiredSourceProjectionIsStateless(t *testing.T) {
	// A component without clients must refuse every old request without any I/O.
	c := &Component{}
	for _, body := range [][]byte{nil, []byte(`{"removal":{}}`), []byte(`not-json`)} {
		raw, err := c.handleSourceProjection(context.Background(), body)
		if err != nil {
			t.Fatal(err)
		}
		var reply struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatal(err)
		}
		if reply.Error.Code != "SOURCE_LIFECYCLE_UNAVAILABLE" {
			t.Fatalf("response=%s", raw)
		}
	}
	_, err := c.runLifecyclePass(context.Background(), graph.LifecycleRunRequest{Reason: graph.LifecycleReasonSourceRemoved})
	if err == nil || !strings.Contains(err.Error(), "SOURCE_LIFECYCLE_UNAVAILABLE") {
		t.Fatalf("legacy route implies a working owner: %v", err)
	}
}
