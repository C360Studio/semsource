package videosource

import "github.com/c360studio/semsource/internal/sourceintent"

// BindSourceLifecycle forwards immutable root admission before producer Start.
func (c *Component) BindSourceLifecycle(binding sourceintent.Binding, observer sourceintent.PublicationObserver) error {
	return c.publisher.BindSourceLifecycle(binding, observer)
}
