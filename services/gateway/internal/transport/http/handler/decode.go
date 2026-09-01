package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/KDZZZZZZ/short-term/platform/errs"
)

// decodeJSON reads a JSON request body into target, rejecting unknown fields
// and trailing content.
//
// The contract marks every request schema additionalProperties: false, so an
// unknown field is a client error rather than something to ignore silently.
// It reports whether decoding succeeded; on failure it has already written the
// response.
func decodeJSON(w http.ResponseWriter, r *http.Request, responder Responder, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			responder.Fail(w, r, errs.CodePayloadTooLarge, "请求内容超过限制")
			return false
		}
		responder.Fail(w, r, errs.CodeValidation, decodeMessage(err))
		return false
	}
	if decoder.More() {
		responder.Fail(w, r, errs.CodeValidation, "请求体包含多余内容")
		return false
	}
	return true
}

// unknownFieldPrefix is the text encoding/json uses for a rejected property.
// There is no typed error for it, so the prefix is matched deliberately here
// rather than at each call site.
const unknownFieldPrefix = "json: unknown field "

// decodeMessage turns a decoding failure into a message that tells the client
// what to fix, without echoing the submitted value back.
func decodeMessage(err error) string {
	message := err.Error()
	if field, found := strings.CutPrefix(message, unknownFieldPrefix); found {
		return "请求体包含未定义的字段 " + field
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		return "字段 " + typeErr.Field + " 的类型不合法"
	}
	return "请求体不是合法的 JSON"
}

// decodeOptionalJSON behaves like decodeJSON but accepts an empty body, which
// the contract allows for requests whose body is not required.
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, responder Responder, target any) bool {
	if r.ContentLength == 0 {
		return true
	}
	return decodeJSON(w, r, responder, target)
}
