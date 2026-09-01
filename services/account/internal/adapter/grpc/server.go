// Package grpc exposes the Account Service use cases over the internal gRPC
// contract in proto/shortterm/account/v1.
//
// The adapter only maps between generated messages and application commands.
// Every authorization rule lives in the application and domain layers, so a
// second transport could not bypass it.
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

// maxBatchUsers bounds one BatchGetUsers call. The Gateway completes profiles
// for one page of results at a time, and the largest public page is 100 rows
// with two distinct participants each.
const maxBatchUsers = 200

// Server adapts application.Service to the generated service interface.
type Server struct {
	accountv1.UnimplementedAccountServiceServer

	app *application.Service
}

// NewServer builds the gRPC adapter.
func NewServer(app *application.Service) *Server { return &Server{app: app} }

// Register creates an account and returns an access token.
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

// Login authenticates a student number and password pair.
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

// GetUser returns one profile including contact details.
func (s *Server) GetUser(ctx context.Context, req *accountv1.GetUserRequest) (*accountv1.GetUserResponse, error) {
	account, err := s.app.GetUser(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	return &accountv1.GetUserResponse{User: userContact(account)}, nil
}

// GetProfile returns the caller's own full profile, including the student
// number. The actor check is what keeps that field private: GetUser, which any
// aggregation path may call, cannot return it at all.
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

// BatchGetUsers returns the public profiles that exist among the requested
// identifiers. Contact details are deliberately not part of this response.
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

// UpdateProfile changes the caller's own profile.
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

// ChangePassword replaces the caller's own password.
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

// requireActor rejects a call whose target user disagrees with the actor the
// caller declared in metadata.
//
// The Gateway fills both from the same verified token, so a mismatch means the
// call did not come from the Gateway's authenticated path. This is defence in
// depth on top of the private network boundary, not a replacement for it:
// internal service identity is an open decision in docs/software-design.md
// section 11.3.
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

// optional copies an optional proto string into an application pointer.
func optional(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

// patch maps the three-state NullableStringPatch onto the application patch
// type: a nil message means the field was absent, null_value means clear it,
// and string_value means set it.
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
