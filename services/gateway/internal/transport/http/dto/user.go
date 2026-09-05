package dto

// UserPublic 是 UserPublic schema，也是列表中唯一使用的身份结构。
type UserPublic struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
}

// UserProfile 是 UserProfile schema：公开用户资料，不含学号和联系方式。
type UserProfile struct {
	ID                   string  `json:"id"`
	Nickname             string  `json:"nickname"`
	AverageScore         *string `json:"average_score"`
	CompletedTradesCount int64   `json:"completed_trades_count"`
	OnSaleProductsCount  int64   `json:"on_sale_products_count"`
}

// SellerContact 是 SellerContact schema。它包含卖家的微信、QQ 和公开平均分，
// 但有意不包含学号字段：已批准契约禁止公开学号（openapi/paths/products.yaml，getProduct）。
type SellerContact struct {
	ID           string  `json:"id"`
	Nickname     string  `json:"nickname"`
	Wechat       *string `json:"wechat"`
	QQ           *string `json:"qq"`
	AverageScore *string `json:"average_score"`
}

// UserMe 是 UserMe schema，只返回给通过认证的所有者。
type UserMe struct {
	ID           string  `json:"id"`
	StudentNo    string  `json:"student_no"`
	Nickname     string  `json:"nickname"`
	Wechat       *string `json:"wechat"`
	QQ           *string `json:"qq"`
	AverageScore *string `json:"average_score"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// AuthData 是注册和登录返回的 AuthData schema。
type AuthData struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	User        UserMe `json:"user"`
}

// RegisterRequest 是 RegisterRequest schema。
type RegisterRequest struct {
	StudentNo string  `json:"student_no"`
	Password  string  `json:"password"`
	Nickname  *string `json:"nickname"`
	Wechat    *string `json:"wechat"`
	QQ        *string `json:"qq"`
}

// LoginRequest 是 LoginRequest schema。
type LoginRequest struct {
	StudentNo string `json:"student_no"`
	Password  string `json:"password"`
}

// UpdateProfileRequest 是 UpdateProfileRequest schema。
//
// 每个字段都是 RawField，使处理器能够区分“缺失”和“null”。响应中的联系方式
// 可以为空，但更新输入不接受 null；省略字段才表示保持不变。
type UpdateProfileRequest struct {
	Nickname RawField `json:"nickname"`
	Wechat   RawField `json:"wechat"`
	QQ       RawField `json:"qq"`
}

// ChangePasswordRequest 是 ChangePasswordRequest schema。
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}
