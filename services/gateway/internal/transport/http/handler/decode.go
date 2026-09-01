package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/KDZZZZZZ/short-term/platform/errs"
)

// decodeJSON 将 JSON 请求正文读取到 target，拒绝未知字段和尾随内容。
//
// 契约将每个请求 schema 标记为 additionalProperties: false，因此未知字段属于客户端
// 错误，而不是可以静默忽略的内容。函数返回解码是否成功；失败时已经写入响应。
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

// unknownFieldPrefix 是 encoding/json 为被拒绝属性使用的文本前缀。
// 由于没有对应的类型化错误，这里有意匹配此前缀，而不是让每个调用点分别处理。
const unknownFieldPrefix = "json: unknown field "

// decodeMessage 将解码失败转换为告知客户端如何修复的消息，同时不回显提交的值。
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

// decodeOptionalJSON 的行为类似 decodeJSON，但接受空正文；契约允许正文非必填的
// 请求这样做。
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, responder Responder, target any) bool {
	if r.ContentLength == 0 {
		return true
	}
	return decodeJSON(w, r, responder, target)
}
