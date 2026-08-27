package s3store

import (
	"errors"
	"fmt"
	"net/url"
)

// DefaultEndpoint is the endpoint a configuration that declares none resolves
// to. AWS is the default because it is the only S3 endpoint that is the same
// for everyone; a self-hosted store always names its own.
const DefaultEndpoint = "https://s3.amazonaws.com"

// Config holds configuration for an S3-compatible object store.
//
// It carries no credential fields, and must not grow any. Runtime
// configuration is watched and replicated through KV, so an access key placed
// here would be distributed well beyond the process that needs it — and strict
// decoding rejects one that shows up anyway, since the struct has nowhere to
// put it. Credentials come from the process environment at construction; see
// New.
type Config struct {
	// Bucket is the bucket every key resolves under. Required.
	Bucket string `json:"bucket" schema:"type:string,description:Bucket holding the objects,category:basic,required:true"`

	// Endpoint is the store's base URL, scheme included — the scheme selects
	// transport security, so https:// connects with TLS and http:// does not.
	// Empty means DefaultEndpoint.
	Endpoint string `json:"endpoint" schema:"type:string,description:S3-compatible endpoint URL (defaults to AWS),category:basic"`

	// Region is forwarded to the store for request signing. Self-hosted
	// stores generally ignore the value; AWS does not. It is accepted and
	// passed through either way rather than being validated against a list,
	// which would make every new AWS region a code change.
	Region string `json:"region" schema:"type:string,description:Region forwarded for request signing,category:basic"`

	// PathStyle selects path-style addressing (endpoint/bucket/key) over
	// virtual-hosted addressing (bucket.endpoint/key). Self-hosted stores
	// reached by IP, or by a hostname with no wildcard DNS record, need it.
	PathStyle bool `json:"path_style" schema:"type:bool,description:Use path-style bucket addressing,category:basic"`
}

// Validate checks that the configuration is complete and consistent. Errors
// name the field and the offending value, because the operator reading them is
// looking at a JSON document, not at this struct.
func (c *Config) Validate() error {
	if c.Bucket == "" {
		return errors.New("bucket is required")
	}
	// An empty endpoint is legal and means DefaultEndpoint, so only a
	// declared one is parsed.
	if c.Endpoint != "" {
		if _, err := parseEndpoint(c.Endpoint); err != nil {
			return fmt.Errorf("endpoint %q: %w", c.Endpoint, err)
		}
	}
	return nil
}

// DefaultConfig returns a Config with sensible defaults. Bucket has no
// sensible default — it is the one value only the operator knows.
func DefaultConfig() Config {
	return Config{
		Endpoint: DefaultEndpoint,
	}
}

// endpoint is an endpoint URL reduced to what the S3 client takes: a host with
// an optional port, and whether to speak TLS.
type endpoint struct {
	host   string
	secure bool
}

// parseEndpoint converts an endpoint URL into its host and transport security.
//
// The checks are deliberately stricter than url.Parse, which accepts almost
// anything: "localhost:9000" parses cleanly into a URL whose scheme is
// "localhost", and silently addressing the wrong host is worse than refusing
// to start.
func parseEndpoint(raw string) (endpoint, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return endpoint{}, fmt.Errorf("not a parseable URL: %w", err)
	}
	switch u.Scheme {
	case "http", "https":
	case "":
		return endpoint{}, errors.New("missing scheme (expected http:// or https://)")
	default:
		return endpoint{}, fmt.Errorf("unsupported scheme %q (expected http:// or https://)", u.Scheme)
	}
	if u.Host == "" {
		return endpoint{}, errors.New("missing host")
	}
	// The client addresses buckets from the host, so an endpoint carrying a
	// base path would be quietly ignored rather than honored.
	if u.Path != "" && u.Path != "/" {
		return endpoint{}, fmt.Errorf("path %q is not supported; the endpoint must be a host", u.Path)
	}
	return endpoint{host: u.Host, secure: u.Scheme == "https"}, nil
}
