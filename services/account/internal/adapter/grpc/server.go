// Package grpc 通过 proto/shortterm/account/v1 中的内部 gRPC 契约暴露
// Account Service 用例。
//
// 适配器只负责在生成消息和应用命令之间映射。
// 所有授权规则都位于应用层和领域层，因此其他传输方式无法绕过这些规则。
package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/account/internal/application"
	"github.com/KDZZZZZZ/short-term/services/account/internal/domain"
)

// maxBatchUsers 限制一次 BatchGetUsers 调用。Gateway 一次为一页结果补全资料，
// 最大公开页面为 100 行，每行最多有两个不同参与者。
const maxBatchUsers = 200

// Server 将 application.Service 适配到生成的服务接口。
type Server struct {
	accountv1.UnimplementedAccountServiceServer

	app *application.Service
}

// NewServer 构造 gRPC 适配器。
func NewServer(app *application.Service) *Server { return &Server{app: app} }

// Register 创建账户并返回访问令牌。
func (s *Server) Register(ctx context.Context, req *accountv1.RegisterRequest) (*accountv1.RegisterResponse, error) {
	result, err := s.app.Register(ctx, application.RegisterCommand{
		StudentNo: req.GetStudentNo(),
		Password:  req.GetPassword(),
		Nickname:  optional(req.Nickname),
		Wechat:    optional(req.Wechat),
		QQ:        optional(req.Qq),
	})
	if err != nil {
		return nil, err
	}
	return &accountv1.RegisterResponse{Auth: authData(result)}, nil
}

// Login 验证学号和密码组合。
func (s *Server) Login(ctx context.Context, req *accountv1.LoginRequest) (*accountv1.LoginResponse, error) {
	result, err := s.app.Login(ctx, application.LoginCommand{
		StudentNo: req.GetStudentNo(),
		Password:  req.GetPassword(),
	})
	if err != nil {
		return nil, err
	}
	return &accountv1.LoginResponse{Auth: authData(result)}, nil
}

// GetUser 返回一个包含联系方式的资料。
func (s *Server) GetUser(ctx context.Context, req *accountv1.GetUserRequest) (*accountv1.GetUserResponse, error) {
	account, err := s.app.GetUser(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	return &accountv1.GetUserResponse{User: userContact(account)}, nil
}

// GetProfile 返回调用方自己的完整资料，包括学号。
// 当前用户检查保证该字段私密：任何聚合路径都可以调用的 GetUser 完全不会返回学号。
func (s *Server) GetProfile(ctx context.Context, req *accountv1.GetProfileRequest) (*accountv1.GetProfileResponse, error) {
	if err := requireActor(ctx, req.GetUserId()); err != nil {
		return nil, err
	}

	account, err := s.app.GetUser(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	return &accountv1.GetProfileResponse{User: userMe(account)}, nil
}

// BatchGetUsers 返回请求标识中存在的公开资料。
// 联系方式有意不包含在此响应中。
func (s *Server) BatchGetUsers(ctx context.Context, req *accountv1.BatchGetUsersRequest) (*accountv1.BatchGetUsersResponse, error) {
	if len(req.GetUserIds()) > maxBatchUsers {
		return nil, errs.Newf(errs.CodeValidation, "单次最多查询 %d 个用户", maxBatchUsers)
	}

	accounts, err := s.app.BatchGetUsers(ctx, req.GetUserIds())
	if err != nil {
		return nil, err
	}

	users := make(map[string]*accountv1.UserPublic, len(accounts))
	for id, account := range accounts {
		users[id] = &accountv1.UserPublic{Id: account.ID, Nickname: account.Nickname}
	}
	return &accountv1.BatchGetUsersResponse{Users: users}, nil
}

// BatchGetUserContacts 返回请求标识中存在的联系方式资料；缺失的用户缺席。
// Gateway 仅用它补全交易双方的联系方式，交易读取端点只对交易方开放。
func (s *Server) BatchGetUserContacts(ctx context.Context, req *accountv1.BatchGetUserContactsRequest) (*accountv1.BatchGetUserContactsResponse, error) {
	if len(req.GetUserIds()) > maxBatchUsers {
		return nil, errs.Newf(errs.CodeValidation, "单次最多查询 %d 个用户", maxBatchUsers)
	}

	accounts, err := s.app.BatchGetUsers(ctx, req.GetUserIds())
	if err != nil {
		return nil, err
	}

	users := make(map[string]*accountv1.UserContact, len(accounts))
	for id, account := range accounts {
		users[id] = &accountv1.UserContact{
			Id:       account.ID,
			Nickname: account.Nickname,
			Wechat:   account.Wechat,
			Qq:       account.QQ,
		}
	}
	return &accountv1.BatchGetUserContactsResponse{Users: users}, nil
}

// UpdateProfile 修改调用方自己的资料。
func (s *Server) UpdateProfile(ctx context.Context, req *accountv1.UpdateProfileRequest) (*accountv1.UpdateProfileResponse, error) {
	if err := requireActor(ctx, req.GetUserId()); err != nil {
		return nil, err
	}

	account, err := s.app.UpdateProfile(ctx, application.UpdateProfileCommand{
		ActorID:  req.GetUserId(),
		Nickname: optional(req.Nickname),
		Wechat:   patch(req.GetWechat()),
		QQ:       patch(req.GetQq()),
	})
	if err != nil {
		return nil, err
	}
	return &accountv1.UpdateProfileResponse{User: userMe(account)}, nil
}

// ChangePassword 替换调用方自己的密码。
func (s *Server) ChangePassword(ctx context.Context, req *accountv1.ChangePasswordRequest) (*accountv1.ChangePasswordResponse, error) {
	if err := requireActor(ctx, req.GetUserId()); err != nil {
		return nil, err
	}

	err := s.app.ChangePassword(ctx, application.ChangePasswordCommand{
		ActorID:     req.GetUserId(),
		OldPassword: req.GetOldPassword(),
		NewPassword: req.GetNewPassword(),
	})
	if err != nil {
		return nil, err
	}
	return &accountv1.ChangePasswordResponse{}, nil
}

// requireActor 拒绝目标用户与调用方在元数据中声明的当前用户不一致的调用。
//
// Gateway 根据同一个已验证令牌填写这两个值，因此不一致意味着调用没有来自 Gateway
// 的认证路径。这是在私有网络边界之上的纵深防御，而不是替代方案：
// 内部服务身份仍是 docs/software-design.md 第 11.3 节中的未决事项。
func requireActor(ctx context.Context, userID string) error {
	if userID == "" {
		return errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if actor := grpcx.ActorID(ctx); actor != "" && actor != userID {
		return errs.New(errs.CodeForbidden, "无权执行该操作")
	}
	return nil
}

func authData(result application.AuthResult) *accountv1.AuthData {
	return &accountv1.AuthData{
		AccessToken: result.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   result.ExpiresIn(),
		User:        userMe(result.Account),
	}
}

func userMe(account *domain.Account) *accountv1.UserMe {
	return &accountv1.UserMe{
		Id:        account.ID,
		StudentNo: account.StudentNo,
		Nickname:  account.Nickname,
		Wechat:    account.Wechat,
		Qq:        account.QQ,
		CreatedAt: timestamppb.New(account.CreatedAt),
		UpdatedAt: timestamppb.New(account.UpdatedAt),
	}
}

func userContact(account *domain.Account) *accountv1.UserContact {
	return &accountv1.UserContact{
		Id:       account.ID,
		Nickname: account.Nickname,
		Wechat:   account.Wechat,
		Qq:       account.QQ,
	}
}

// optional 将可选 proto 字符串复制到应用层指针。
func optional(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

// patch 将线路 patch 映射为应用层类型：nil 表示缺失，string_value 表示设置；
// null_value 被保留下来交给应用层明确拒绝。
func patch(value *accountv1.NullableStringPatch) application.StringPatch {
	if value == nil {
		return application.Keep()
	}
	switch inner := value.GetValue().(type) {
	case *accountv1.NullableStringPatch_StringValue:
		return application.Set(inner.StringValue)
	case *accountv1.NullableStringPatch_NullValue:
		return application.Clear()
	default:
		return application.Keep()
	}
}
