// Package mapper 在内部 gRPC 消息和公开 HTTP DTO 之间进行转换。
// 显式维护转换关系可以避免 Protobuf 变更静默改变已批准的 REST 契约
// （docs/software-design.md 第 7.1 节）。
package mapper

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
)

// AuthData 将内部认证载荷映射为公开 AuthData schema。
func AuthData(src *accountv1.AuthData, averageScore *string) dto.AuthData {
	return dto.AuthData{
		AccessToken: src.GetAccessToken(),
		TokenType:   src.GetTokenType(),
		ExpiresIn:   src.GetExpiresIn(),
		User:        UserMe(src.GetUser(), averageScore),
	}
}

// UserMe 映射调用方自己的资料。averageScore 是 Marketplace 聚合的公开平均分。
func UserMe(src *accountv1.UserMe, averageScore *string) dto.UserMe {
	return dto.UserMe{
		ID:           src.GetId(),
		StudentNo:    src.GetStudentNo(),
		Nickname:     src.GetNickname(),
		Wechat:       src.Wechat,
		QQ:           src.Qq,
		AverageScore: averageScore,
		CreatedAt:    Timestamp(src.GetCreatedAt()),
		UpdatedAt:    Timestamp(src.GetUpdatedAt()),
	}
}

// SellerContact 为商品详情响应映射卖家资料。源消息没有学号字段，
// 因此该映射不会泄露学号。averageScore 是 Marketplace 聚合的公开平均分。
func SellerContact(src *accountv1.UserContact, averageScore *string) dto.SellerContact {
	return dto.SellerContact{
		ID:           src.GetId(),
		Nickname:     src.GetNickname(),
		Wechat:       src.Wechat,
		QQ:           src.Qq,
		AverageScore: averageScore,
	}
}

// UserPublic 映射公开资料。账户缺失时仍返回格式正确的对象：
// 列表响应不能因为某个参与者被删除而失败，且契约要求该字段存在。
func UserPublic(id string, src *accountv1.UserPublic) dto.UserPublic {
	if src == nil {
		return dto.UserPublic{ID: id, Nickname: deletedUserNickname}
	}
	return dto.UserPublic{ID: src.GetId(), Nickname: src.GetNickname()}
}

// deletedUserNickname 代表已不存在的账户。
const deletedUserNickname = "已注销用户"

// Timestamp 将 protobuf 时间戳渲染为契约 date-time 格式要求的 RFC 3339 字符串。
func Timestamp(src *timestamppb.Timestamp) string {
	if src == nil {
		return ""
	}
	return src.AsTime().UTC().Format(time.RFC3339Nano)
}

// OptionalTimestamp 渲染可为空的 date-time 字段。
func OptionalTimestamp(src *timestamppb.Timestamp) *string {
	if src == nil {
		return nil
	}
	value := Timestamp(src)
	return &value
}
