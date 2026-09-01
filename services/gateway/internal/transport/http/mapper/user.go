// Package mapper converts between the internal gRPC messages and the public
// HTTP DTOs. Keeping the conversion explicit is what stops a Protobuf change
// from silently altering the approved REST contract
// (docs/software-design.md section 7.1).
package mapper

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
)

// AuthData maps the internal auth payload to the public AuthData schema.
func AuthData(src *accountv1.AuthData) dto.AuthData {
	return dto.AuthData{
		AccessToken: src.GetAccessToken(),
		TokenType:   src.GetTokenType(),
		ExpiresIn:   src.GetExpiresIn(),
		User:        UserMe(src.GetUser()),
	}
}

// UserMe maps the caller's own profile.
func UserMe(src *accountv1.UserMe) dto.UserMe {
	return dto.UserMe{
		ID:        src.GetId(),
		StudentNo: src.GetStudentNo(),
		Nickname:  src.GetNickname(),
		Wechat:    src.Wechat,
		QQ:        src.Qq,
		CreatedAt: Timestamp(src.GetCreatedAt()),
		UpdatedAt: Timestamp(src.GetUpdatedAt()),
	}
}

// SellerContact maps a seller profile for the product detail response. The
// source message has no student number field, so this mapping cannot leak one.
func SellerContact(src *accountv1.UserContact) dto.SellerContact {
	return dto.SellerContact{
		ID:       src.GetId(),
		Nickname: src.GetNickname(),
		Wechat:   src.Wechat,
		QQ:       src.Qq,
	}
}

// UserPublic maps a public profile. A missing account still yields a
// well-formed object: list responses must not fail because one participant was
// removed, and the contract requires the field to be present.
func UserPublic(id string, src *accountv1.UserPublic) dto.UserPublic {
	if src == nil {
		return dto.UserPublic{ID: id, Nickname: deletedUserNickname}
	}
	return dto.UserPublic{ID: src.GetId(), Nickname: src.GetNickname()}
}

// deletedUserNickname stands in for an account that no longer exists.
const deletedUserNickname = "已注销用户"

// Timestamp renders a protobuf timestamp as the RFC 3339 string the contract's
// date-time format requires.
func Timestamp(src *timestamppb.Timestamp) string {
	if src == nil {
		return ""
	}
	return src.AsTime().UTC().Format(time.RFC3339Nano)
}

// OptionalTimestamp renders a nullable date-time field.
func OptionalTimestamp(src *timestamppb.Timestamp) *string {
	if src == nil {
		return nil
	}
	value := Timestamp(src)
	return &value
}
