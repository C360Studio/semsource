package codecontext

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go"
)

const fusionHTTPErrorContractVersion = "1"

type errorOrigin string

const (
	errorOriginRequest    errorOrigin = "request"
	errorOriginDependency errorOrigin = "dependency"
	errorOriginLocal      errorOrigin = "local"
)

type fusionHTTPError struct {
	status    int
	code      string
	class     string
	message   string
	retryable bool
}

type fusionHTTPErrorEnvelope struct {
	Error fusionHTTPErrorBody `json:"error"`
}

type fusionHTTPErrorBody struct {
	ContractVersion string `json:"contract_version"`
	Code            string `json:"code"`
	Class           string `json:"class"`
	Message         string `json:"message"`
	Retryable       bool   `json:"retryable"`
}

func classifyFusionHTTPError(origin errorOrigin, err error) fusionHTTPError {
	if origin == errorOriginRequest {
		return invalidRequestError()
	}
	if origin == errorOriginLocal {
		return fusionHTTPError{
			status:  http.StatusInternalServerError,
			code:    "internal_error",
			class:   "fatal",
			message: "the fusion request could not be completed",
		}
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, nats.ErrTimeout) {
		return fusionHTTPError{
			status:    http.StatusGatewayTimeout,
			code:      "upstream_timeout",
			class:     "transient",
			message:   "graph query service timed out",
			retryable: true,
		}
	}
	var classified *errs.ClassifiedError
	isClassified := errors.As(err, &classified)
	if natsclient.IsNoResponders(err) || errors.Is(err, natsclient.ErrNotConnected) ||
		errors.Is(err, natsclient.ErrCircuitOpen) ||
		(isClassified && classified.Class == errs.ErrorTransient) {
		return fusionHTTPError{
			status:    http.StatusServiceUnavailable,
			code:      "dependency_unavailable",
			class:     "transient",
			message:   "graph query service is temporarily unavailable",
			retryable: true,
		}
	}
	if isClassified && classified.Class == errs.ErrorInvalid {
		return fusionHTTPError{
			status:  http.StatusBadGateway,
			code:    "upstream_contract_error",
			class:   "fatal",
			message: "graph query service rejected an internal request",
		}
	}
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return fusionHTTPError{
			status:  http.StatusBadGateway,
			code:    "upstream_contract_error",
			class:   "fatal",
			message: "graph query service returned an invalid response",
		}
	}
	return fusionHTTPError{
		status:  http.StatusBadGateway,
		code:    "upstream_failure",
		class:   "fatal",
		message: "graph query service failed",
	}
}

func methodNotAllowedError() fusionHTTPError {
	return fusionHTTPError{
		status:  http.StatusMethodNotAllowed,
		code:    "method_not_allowed",
		class:   "invalid",
		message: "method not allowed",
	}
}

func requestTooLargeError() fusionHTTPError {
	return fusionHTTPError{
		status:  http.StatusRequestEntityTooLarge,
		code:    "request_too_large",
		class:   "invalid",
		message: "request body exceeds the allowed size",
	}
}

func invalidJSONError() fusionHTTPError {
	return fusionHTTPError{
		status:  http.StatusBadRequest,
		code:    "invalid_json",
		class:   "invalid",
		message: "request body must be valid JSON",
	}
}

func invalidRequestError() fusionHTTPError {
	return fusionHTTPError{
		status:  http.StatusBadRequest,
		code:    "invalid_request",
		class:   "invalid",
		message: "query must not be blank",
	}
}

func componentNotReadyError() fusionHTTPError {
	return fusionHTTPError{
		status:    http.StatusServiceUnavailable,
		code:      "component_not_ready",
		class:     "transient",
		message:   "fusion service is not ready",
		retryable: true,
	}
}

func writeFusionHTTPError(w http.ResponseWriter, public fusionHTTPError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(public.status)
	_ = json.NewEncoder(w).Encode(fusionHTTPErrorEnvelope{Error: fusionHTTPErrorBody{
		ContractVersion: fusionHTTPErrorContractVersion,
		Code:            public.code,
		Class:           public.class,
		Message:         public.message,
		Retryable:       public.retryable,
	}})
}
