package sourcemanifest

import "net/http"

func (c *Component) handleLifecycleHTTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := c.authorizedIngest(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusGone, struct {
		Error *IngestError `json:"error"`
	}{
		Error: &IngestError{Code: "SOURCE_LIFECYCLE_UNAVAILABLE", Message: "source lifecycle projection is unavailable"},
	})
}
