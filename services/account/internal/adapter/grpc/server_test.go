package grpc_test

import (
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	"github.com/KDZZZZZZ/short-term/platform/auth"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/platform/pgtest"
	grpcadapter "github.com/KDZZZZZZ/short-term/services/account/internal/adapter/grpc"
	"github.com/KDZZZZZZ/short-term/services/account/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/account/internal/adapter/system"
	"github.com/KDZZZZZZ/short-term/services/account/internal/application"
	"github.com/KDZZZZZZ/short-term/services/account/migrations"
)

// newClient starts the real gRPC adapter over the real repository, the real
// Argon2id hasher and the real token issuer, and returns a client for it.
// Only the network is local; nothing in the chain is stubbed, so this is the
// evidence that registration, login and profile access work end to end inside
// the service.
func newClient(t *testing.T) (accountv1.AccountServiceClient, *auth.Verifier) {
	t.Helper()

	pool := pgtest.New(t, migrations.FS, migrations.Dir)

	hasher, err := auth.NewHasher(auth.DefaultArgon2Params())
	if err != nil {
		t.Fatalf("NewHasher: %v", err)
	}
	tokenCfg := auth.Config{
		SigningKey: []byte(strings.Repeat("test-signing-key", 2)),
		Issuer:     "shortterm-account",
		Audience:   "shortterm-api",
		TTL:        time.Hour,
		Leeway:     time.Second,
	}
	ids := system.NewIDs()
	issuer, err := auth.NewIssuer(tokenCfg, nil, func() string { return ids.New() })
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	verifier, err := auth.NewVerifier(tokenCfg, nil)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	app, err := application.NewService(
		postgres.NewAccountRepository(pool),
		hasher,
		issuer,
		ids,
		system.Clock{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpcx.NewServer(grpcx.ServerOptions{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		HandlerTimeout: 10 * time.Second,
	})
	accountv1.RegisterAccountServiceServer(server, grpcadapter.NewServer(app))

	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpcx.Dial(grpcx.ClientOptions{
		Target:         listener.Addr().String(),
		Caller:         "test",
		DefaultTimeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return accountv1.NewAccountServiceClient(conn), verifier
}

func TestRegisterThenLoginThenReadProfile(t *testing.T) {
	t.Parallel()

	client, verifier := newClient(t)
	ctx := t.Context()

	registered, err := client.Register(ctx, &accountv1.RegisterRequest{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
		Wechat:    strptr("wx_xiaoming"),
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	auth := registered.GetAuth()
	if auth.GetTokenType() != "Bearer" {
		t.Fatalf("token_type = %q, want Bearer", auth.GetTokenType())
	}
	if auth.GetExpiresIn() != 3600 {
		t.Fatalf("expires_in = %d, want 3600", auth.GetExpiresIn())
	}

	claims, err := verifier.Verify(auth.GetAccessToken())
	if err != nil {
		t.Fatalf("the issued token does not verify: %v", err)
	}
	if claims.Subject != auth.GetUser().GetId() {
		t.Fatalf("token subject = %q, want the account id %q", claims.Subject, auth.GetUser().GetId())
	}

	loggedIn, err := client.Login(ctx, &accountv1.LoginRequest{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if loggedIn.GetAuth().GetUser().GetId() != auth.GetUser().GetId() {
		t.Fatal("Login returned a different account")
	}

	got, err := client.GetUser(ctx, &accountv1.GetUserRequest{UserId: auth.GetUser().GetId()})
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.GetUser().GetWechat() != "wx_xiaoming" {
		t.Fatalf("wechat = %q", got.GetUser().GetWechat())
	}
}

func TestRegisterRejectsADuplicateStudentNumber(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t)
	req := &accountv1.RegisterRequest{StudentNo: "20260001", Password: "correct-horse-battery"}

	if _, err := client.Register(t.Context(), req); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	_, err := client.Register(t.Context(), req)
	assertCode(t, err, errs.CodeStudentNoExists)
}

func TestLoginFailsWithTheWrongPassword(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t)
	if _, err := client.Register(t.Context(), &accountv1.RegisterRequest{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := client.Login(t.Context(), &accountv1.LoginRequest{
		StudentNo: "20260001",
		Password:  "wrong-password-here",
	})
	assertCode(t, err, errs.CodeUnauthorized)
}

func TestBatchGetUsersReturnsPublicProfilesOnly(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t)
	first := registerAccount(t, client, "20260001")
	second := registerAccount(t, client, "20260002")

	resp, err := client.BatchGetUsers(t.Context(), &accountv1.BatchGetUsersRequest{
		UserIds: []string{first, second, "u_missing"},
	})
	if err != nil {
		t.Fatalf("BatchGetUsers: %v", err)
	}
	if len(resp.GetUsers()) != 2 {
		t.Fatalf("got %d users, want 2", len(resp.GetUsers()))
	}

	// UserPublic has no contact or student number fields at all, which is the
	// point: list aggregation cannot leak them even by accident.
	for id, user := range resp.GetUsers() {
		if user.GetId() != id {
			t.Fatalf("map key %q does not match user id %q", id, user.GetId())
		}
		if user.GetNickname() == "" {
			t.Fatal("a public profile has no nickname")
		}
	}
}

func TestBatchGetUsersRejectsAnOversizedRequest(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t)
	ids := make([]string, 201)
	for i := range ids {
		ids[i] = "u_x"
	}

	_, err := client.BatchGetUsers(t.Context(), &accountv1.BatchGetUsersRequest{UserIds: ids})
	assertCode(t, err, errs.CodeValidation)
}

func TestUpdateProfileRejectsAMismatchedActor(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t)
	victim := registerAccount(t, client, "20260001")
	attacker := registerAccount(t, client, "20260002")

	ctx := grpcx.WithActor(t.Context(), attacker)
	_, err := client.UpdateProfile(ctx, &accountv1.UpdateProfileRequest{
		UserId:   victim,
		Nickname: strptr("被改掉的昵称"),
	})
	assertCode(t, err, errs.CodeForbidden)
}

func TestUpdateProfileClearsAndSetsContacts(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t)
	ctx := t.Context()

	registered, err := client.Register(ctx, &accountv1.RegisterRequest{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
		Wechat:    strptr("wx_xiaoming"),
		Qq:        strptr("123456789"),
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	userID := registered.GetAuth().GetUser().GetId()

	updated, err := client.UpdateProfile(grpcx.WithActor(ctx, userID), &accountv1.UpdateProfileRequest{
		UserId: userID,
		Wechat: &accountv1.NullableStringPatch{Value: &accountv1.NullableStringPatch_NullValue{}},
		Qq:     &accountv1.NullableStringPatch{Value: &accountv1.NullableStringPatch_StringValue{StringValue: "987654321"}},
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	if updated.GetUser().Wechat != nil {
		t.Fatalf("wechat = %q, want cleared", updated.GetUser().GetWechat())
	}
	if updated.GetUser().GetQq() != "987654321" {
		t.Fatalf("qq = %q, want 987654321", updated.GetUser().GetQq())
	}
	if updated.GetUser().GetStudentNo() != "20260001" {
		t.Fatalf("student_no = %q, want it unchanged", updated.GetUser().GetStudentNo())
	}
}

func TestChangePasswordThenLoginWithTheNewPassword(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t)
	ctx := t.Context()
	userID := registerAccount(t, client, "20260001")

	if _, err := client.ChangePassword(grpcx.WithActor(ctx, userID), &accountv1.ChangePasswordRequest{
		UserId:      userID,
		OldPassword: "correct-horse-battery",
		NewPassword: "a-brand-new-password",
	}); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	if _, err := client.Login(ctx, &accountv1.LoginRequest{
		StudentNo: "20260001",
		Password:  "a-brand-new-password",
	}); err != nil {
		t.Fatalf("login with the new password: %v", err)
	}
	_, err := client.Login(ctx, &accountv1.LoginRequest{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
	})
	assertCode(t, err, errs.CodeUnauthorized)
}

func TestErrorsCarryNoSensitiveDetail(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t)
	const secret = "super-secret-password"

	_, err := client.Login(t.Context(), &accountv1.LoginRequest{StudentNo: "20260001", Password: secret})
	if err == nil {
		t.Fatal("expected a login failure")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("the gRPC error leaked the password: %v", err)
	}
}

func registerAccount(t *testing.T, client accountv1.AccountServiceClient, studentNo string) string {
	t.Helper()

	resp, err := client.Register(t.Context(), &accountv1.RegisterRequest{
		StudentNo: studentNo,
		Password:  "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Register(%s): %v", studentNo, err)
	}
	return resp.GetAuth().GetUser().GetId()
}

func assertCode(t *testing.T, err error, want errs.Code) {
	t.Helper()

	if err == nil {
		t.Fatalf("got no error, want %s", want)
	}
	if got := errs.CodeOf(err); got != want {
		t.Fatalf("error code = %s, want %s (%v)", got, want, err)
	}
}

func strptr(value string) *string { return &value }
