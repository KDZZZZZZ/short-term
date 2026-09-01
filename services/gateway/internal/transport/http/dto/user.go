package dto

// UserPublic is the UserPublic schema: the only identity shape used in lists.
type UserPublic struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
}

// SellerContact is the SellerContact schema. It carries the seller's WeChat
// and QQ but deliberately has no student number field: the approved contract
// forbids disclosing it (openapi/paths/products.yaml, getProduct).
type SellerContact struct {
	ID       string  `json:"id"`
	Nickname string  `json:"nickname"`
	Wechat   *string `json:"wechat"`
	QQ       *string `json:"qq"`
}

// UserMe is the UserMe schema, returned only to the authenticated owner.
type UserMe struct {
	ID        string  `json:"id"`
	StudentNo string  `json:"student_no"`
	Nickname  string  `json:"nickname"`
	Wechat    *string `json:"wechat"`
	QQ        *string `json:"qq"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// AuthData is the AuthData schema returned by register and login.
type AuthData struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	User        UserMe `json:"user"`
}

// RegisterRequest is the RegisterRequest schema.
type RegisterRequest struct {
	StudentNo string  `json:"student_no"`
	Password  string  `json:"password"`
	Nickname  *string `json:"nickname"`
	Wechat    *string `json:"wechat"`
	QQ        *string `json:"qq"`
}

// LoginRequest is the LoginRequest schema.
type LoginRequest struct {
	StudentNo string `json:"student_no"`
	Password  string `json:"password"`
}

// UpdateProfileRequest is the UpdateProfileRequest schema.
//
// Each field is a json.RawMessage so the handler can tell "absent" from
// "null": the contract's Wechat and QQ schemas are nullable, and clearing a
// contact is a different request from leaving it alone.
type UpdateProfileRequest struct {
	Nickname RawField `json:"nickname"`
	Wechat   RawField `json:"wechat"`
	QQ       RawField `json:"qq"`
}

// ChangePasswordRequest is the ChangePasswordRequest schema.
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}
