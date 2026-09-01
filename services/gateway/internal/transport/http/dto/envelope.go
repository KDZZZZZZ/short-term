// Package dto 保存公开 API 的 JSON 结构。这里的每个类型都对应
// openapi/components/schemas.yaml 中的一个 schema；这里不能携带 gRPC、数据库或领域逻辑。
package dto

// SuccessEnvelope 是 SuccessBase schema：包含固定 code 和 message，以及各端点的数据载荷。
type SuccessEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// ErrorEnvelope 是 ErrorResponse schema。Details 会被省略而不是发送 null；
// schema 允许这样做，也能避免错误正文携带调用方不需要的信息。
type ErrorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}
