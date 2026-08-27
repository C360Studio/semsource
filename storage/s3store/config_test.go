package s3store_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/c360studio/semsource/storage/s3store"
)

func TestValidate_RejectsEmptyBucket(t *testing.T) {
	cfg := s3store.Config{Endpoint: "https://s3.example.com"}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected an error for a config with no bucket")
	}
	if !strings.Contains(err.Error(), "bucket") {
		t.Errorf("error should name the field, got: %v", err)
	}
}

func TestValidate_RejectsUnparseableEndpoint(t *testing.T) {
	// Each case is a way an endpoint can be wrong that url.Parse alone would
	// wave through, which is why parseEndpoint checks more than parseability.
	cases := []struct {
		name     string
		endpoint string
	}{
		{"no scheme, host:port only", "localhost:9000"},
		{"scheme separator with no scheme", "://storage.example.com"},
		{"unsupported scheme", "ftp://storage.example.com"},
		{"scheme with no host", "https://"},
		{"endpoint carrying a base path", "https://storage.example.com/s3/v1"},
		{"space in the host", "https://storage example.com"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := s3store.Config{Bucket: "artifacts", Endpoint: tc.endpoint}

			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected an error for endpoint %q", tc.endpoint)
			}
			// The operator is reading a JSON document, so the message has to
			// name both the field and what they wrote in it.
			msg := err.Error()
			if !strings.Contains(msg, "endpoint") {
				t.Errorf("error should name the field, got: %v", err)
			}
			if !strings.Contains(msg, tc.endpoint) {
				t.Errorf("error should quote the offending value %q, got: %v", tc.endpoint, err)
			}
		})
	}
}

func TestValidate_AcceptsValidEndpoints(t *testing.T) {
	cases := []struct {
		name string
		cfg  s3store.Config
	}{
		{"no endpoint means the default", s3store.Config{Bucket: "artifacts"}},
		{"AWS", s3store.Config{Bucket: "artifacts", Endpoint: s3store.DefaultEndpoint}},
		{"self-hosted over TLS", s3store.Config{
			Bucket:    "artifacts",
			Endpoint:  "https://garage.internal:3900",
			Region:    "garage",
			PathStyle: true,
		}},
		{"self-hosted without TLS", s3store.Config{
			Bucket:    "artifacts",
			Endpoint:  "http://127.0.0.1:9000",
			PathStyle: true,
		}},
		{"trailing slash", s3store.Config{Bucket: "artifacts", Endpoint: "https://storage.example.com/"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := s3store.DefaultConfig()

	if cfg.Endpoint != s3store.DefaultEndpoint {
		t.Errorf("endpoint = %q, want %q", cfg.Endpoint, s3store.DefaultEndpoint)
	}
	if cfg.PathStyle {
		t.Error("path style should default off — AWS uses virtual-hosted addressing")
	}
	// Bucket is the one value only the operator knows, so the default config
	// must not validate: there is nothing sensible to put there.
	if err := cfg.Validate(); err == nil {
		t.Error("the default config should not validate without a bucket")
	}
}

// TestStrictDecodeRejectsCredentialFields is the structural half of the
// no-credentials-in-configuration requirement. The struct has nowhere to put a
// key, so strict decoding rejects one as an ordinary unknown field — the same
// classification any other typo gets, with no credential-specific code to
// maintain or forget.
func TestStrictDecodeRejectsCredentialFields(t *testing.T) {
	fields := []string{
		"access_key_id",
		"secret_access_key",
		"aws_access_key_id",
		"aws_secret_access_key",
		"session_token",
		"credentials",
	}

	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			doc := `{"bucket":"artifacts","` + field + `":"AKIAIOSFODNN7EXAMPLE"}`

			dec := json.NewDecoder(strings.NewReader(doc))
			dec.DisallowUnknownFields()

			var cfg s3store.Config
			err := dec.Decode(&cfg)
			if err == nil {
				t.Fatalf("strict decoding accepted a %q field", field)
			}
			if !strings.Contains(err.Error(), field) {
				t.Errorf("error should name the rejected field, got: %v", err)
			}
		})
	}
}

// TestValidateOutputCarriesNoSecret guards the reporting half: whatever an
// operator sees from validation identifies the store by endpoint and bucket
// and nothing else. It holds trivially today because Config has no credential
// field — which is the point. The test fails the moment one is added.
func TestValidateOutputCarriesNoSecret(t *testing.T) {
	const secret = "wJalrXUtnFEMI-K7MDENG-bPxRfiCYEXAMPLEKEY"
	t.Setenv(s3store.EnvAccessKeyID, "AKIAIOSFODNN7EXAMPLE")
	t.Setenv(s3store.EnvSecretAccessKey, secret)

	cfg := s3store.Config{Endpoint: "not a url", Region: "garage"}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected an error")
	}
	// %+v renders every field, which is how a config reaches a log line by
	// accident. Both that and the validation error have to come out clean.
	rendered := err.Error() + " " + fmt.Sprintf("%+v", cfg)
	if strings.Contains(rendered, secret) {
		t.Errorf("validation output leaked a credential: %s", rendered)
	}
}
