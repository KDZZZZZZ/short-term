// Package dto holds the JSON shapes of the public API. Every type here mirrors
// a schema in openapi/components/schemas.yaml; nothing here may carry gRPC,
// database or domain concerns.
package dto

// SuccessEnvelope is the SuccessBase schema: a fixed code and message with a
// per-endpoint data payload.
type SuccessEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// ErrorEnvelope is the ErrorResponse schema. Details are omitted rather than
// sent as null, which the schema permits and which keeps error bodies from
// carrying anything a caller did not need.
type ErrorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}
