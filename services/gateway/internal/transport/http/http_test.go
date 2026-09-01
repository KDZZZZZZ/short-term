package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	"github.com/KDZZZZZZ/short-term/platform/auth"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	gatewayhttp "github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/handler"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

const (
	basePath  = "/api/v1"
	testActor = "u_01ARZ3NDEKTSV4RRFFQ69G5FAV"
)

func tokenConfig() auth.Config {
	return auth.Config{
		SigningKey: []byte(strings.Repeat("gateway-test-key", 2)),
		Issuer:     "shortterm-account",
		Audience:   "shortterm-api",
		TTL:        time.Hour,
		Leeway:     time.Second,
	}
}

func newServer(t *testing.T, accounts accountv1.AccountServiceClient) (*httptest.Server, string) {
	t.Helper()

	verifier, err := auth.NewVerifier(tokenConfig(), nil)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	issuer, err := auth.NewIssuer(tokenConfig(), nil, func() string { return "jti" })
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	token, _, err := issuer.Issue(testActor)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := gatewayhttp.NewResponder(logger)
	router := gatewayhttp.NewRouter(gatewayhttp.RouterOptions{
		BasePath:     basePath,
		Verifier:     verifier,
		MaxBodyBytes: 1 << 20,
		Logger:       logger,
		Ready:        func(context.Context) error { return nil },
		Auth:         handler.NewAuth(accounts, responder),
		Users:        handler.NewUsers(accounts, responder),
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, token
}

func TestRegisterReturns201WithTheAuthEnvelope(t *testing.T) {
	t.Parallel()

	server, _ := newServer(t, &stubAccounts{})

	status, body := request(t, server, http.MethodPost, basePath+"/auth/register", "",
		`{"student_no":"20260001","password":"correct-horse-battery"}`)

	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", status, body)
	}

	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			ExpiresIn   int64  `json:"expires_in"`
			User        struct {
				ID        string `json:"id"`
				StudentNo string `json:"student_no"`
			} `json:"user"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if envelope.Code != "OK" || envelope.Message != "success" {
		t.Fatalf("envelope = %+v, want the SuccessBase constants", envelope)
	}
	if envelope.Data.TokenType != "Bearer" {
		t.Fatalf("token_type = %q, want Bearer", envelope.Data.TokenType)
	}
	if envelope.Data.ExpiresIn <= 0 {
		t.Fatalf("expires_in = %d, want a positive lifetime", envelope.Data.ExpiresIn)
	}
	if envelope.Data.User.ID == "" || envelope.Data.User.StudentNo == "" {
		t.Fatalf("AuthData.user is incomplete: %s", body)
	}
}

func TestLoginReturns200(t *testing.T) {
	t.Parallel()

	server, _ := newServer(t, &stubAccounts{})

	status, body := request(t, server, http.MethodPost, basePath+"/auth/login", "",
		`{"student_no":"20260001","password":"correct-horse-battery"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
}

func TestProtectedRoutesRequireABearerToken(t *testing.T) {
	t.Parallel()

	server, token := newServer(t, &stubAccounts{})

	tests := []struct {
		name  string
		token string
	}{
		{name: "no header", token: ""},
		{name: "wrong scheme", token: "Basic " + token},
		{name: "empty bearer", token: "Bearer "},
		{name: "garbage token", token: "Bearer not-a-token"},
		{name: "token from another issuer", token: "Bearer " + foreignToken(t)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body := requestRaw(t, server, http.MethodGet, basePath+"/users/me", tt.token, "")
			if status != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401: %s", status, body)
			}
			assertErrorCode(t, body, errs.CodeUnauthorized)
		})
	}
}

func TestGetMeReturnsTheOwnersProfile(t *testing.T) {
	t.Parallel()

	server, token := newServer(t, &stubAccounts{})

	status, body := request(t, server, http.MethodGet, basePath+"/users/me", token, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	var envelope struct {
		Data struct {
			ID        string  `json:"id"`
			StudentNo string  `json:"student_no"`
			Nickname  string  `json:"nickname"`
			Wechat    *string `json:"wechat"`
			QQ        *string `json:"qq"`
			CreatedAt string  `json:"created_at"`
			UpdatedAt string  `json:"updated_at"`
		} `json:"data"`
	}
	decode(t, body, &envelope)

	if envelope.Data.ID != testActor {
		t.Fatalf("id = %q, want the token subject %q", envelope.Data.ID, testActor)
	}
	if envelope.Data.StudentNo == "" {
		t.Fatal("the owner's own profile must include the student number")
	}
	// UserMe 要求每个属性都存在，包括可为空的属性。
	for _, field := range []string{"wechat", "qq", "created_at", "updated_at"} {
		if !strings.Contains(body, `"`+field+`"`) {
			t.Fatalf("response omits the required field %q: %s", field, body)
		}
	}
}

func TestPatchMeSendsOmittedAndSetPatchesDownstream(t *testing.T) {
	t.Parallel()

	accounts := &stubAccounts{}
	server, token := newServer(t, accounts)

	status, body := request(t, server, http.MethodPatch, basePath+"/users/me", token,
		`{"wechat":"wx_updated","qq":"987654321"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}

	req := accounts.lastUpdate
	if req == nil {
		t.Fatal("UpdateProfile was not called")
	}
	if req.Nickname != nil {
		t.Fatal("an absent nickname must not be sent downstream")
	}
	wechat, isSet := req.GetWechat().GetValue().(*accountv1.NullableStringPatch_StringValue)
	if !isSet || wechat.StringValue != "wx_updated" {
		t.Fatalf("wechat patch = %v, want wx_updated", req.GetWechat())
	}
	value, isSet := req.GetQq().GetValue().(*accountv1.NullableStringPatch_StringValue)
	if !isSet || value.StringValue != "987654321" {
		t.Fatalf("qq patch = %v, want 987654321", req.GetQq())
	}
	if req.GetUserId() != testActor {
		t.Fatalf("user_id = %q, want the token subject", req.GetUserId())
	}
}

func TestPatchMeRejectsInvalidBodies(t *testing.T) {
	t.Parallel()

	server, token := newServer(t, &stubAccounts{})

	tests := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"student_no":"20260002"}`},
		{name: "empty object", body: `{}`},
		{name: "null nickname", body: `{"nickname":null}`},
		{name: "null wechat", body: `{"wechat":null}`},
		{name: "null qq", body: `{"qq":null}`},
		{name: "non-string wechat", body: `{"wechat":42}`},
		{name: "not json", body: `nope`},
		{name: "trailing content", body: `{"nickname":"a"}{"nickname":"b"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body := request(t, server, http.MethodPatch, basePath+"/users/me", token, tt.body)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", status, body)
			}
			assertErrorCode(t, body, errs.CodeValidation)
		})
	}
}

func TestChangePasswordMapsDownstreamCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   errs.Code
	}{
		{name: "success", err: nil, wantStatus: http.StatusOK, wantCode: ""},
		{name: "wrong current password", err: errs.New(errs.CodeUnauthorized, "当前密码错误"), wantStatus: http.StatusUnauthorized, wantCode: errs.CodeUnauthorized},
		{name: "invalid new password", err: errs.New(errs.CodeValidation, "新密码长度不合法"), wantStatus: http.StatusBadRequest, wantCode: errs.CodeValidation},
		{name: "downstream failure", err: errs.New(errs.CodeInternal, "boom"), wantStatus: http.StatusInternalServerError, wantCode: errs.CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, token := newServer(t, &stubAccounts{changePasswordErr: tt.err})
			status, body := request(t, server, http.MethodPut, basePath+"/users/me/password", token,
				`{"old_password":"correct-horse-battery","new_password":"a-brand-new-password"}`)

			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", status, tt.wantStatus, body)
			}
			if tt.wantCode != "" {
				assertErrorCode(t, body, tt.wantCode)
			}
		})
	}
}

func TestInternalErrorsDoNotLeakDownstreamDetail(t *testing.T) {
	t.Parallel()

	server, token := newServer(t, &stubAccounts{
		changePasswordErr: errs.Wrap(errs.CodeInternal, "pq: relation \"accounts\" does not exist", nil),
	})

	_, body := request(t, server, http.MethodPut, basePath+"/users/me/password", token,
		`{"old_password":"correct-horse-battery","new_password":"a-brand-new-password"}`)

	if strings.Contains(body, "relation") || strings.Contains(body, "pq:") {
		t.Fatalf("the response leaked a downstream failure detail: %s", body)
	}
	assertErrorCode(t, body, errs.CodeInternal)
}

func TestUnknownApiPathReturnsTheContractErrorEnvelope(t *testing.T) {
	t.Parallel()

	server, token := newServer(t, &stubAccounts{})

	status, body := request(t, server, http.MethodGet, basePath+"/nope", token, "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", status, body)
	}
	assertErrorCode(t, body, errs.CodeResourceNotFound)
}

func TestOversizedBodyIsRejected(t *testing.T) {
	t.Parallel()

	server, _ := newServer(t, &stubAccounts{})
	huge := `{"student_no":"20260001","password":"` + strings.Repeat("x", 2<<20) + `"}`

	status, body := request(t, server, http.MethodPost, basePath+"/auth/register", "", huge)
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %s", status, body)
	}
	assertErrorCode(t, body, errs.CodePayloadTooLarge)
}

func TestHealthEndpointsSitOutsideTheVersionedApi(t *testing.T) {
	t.Parallel()

	server, _ := newServer(t, &stubAccounts{})

	for _, path := range []string{"/healthz", "/readyz"} {
		status, body := requestRaw(t, server, http.MethodGet, path, "", "")
		if status != http.StatusOK {
			t.Fatalf("%s status = %d, want 200: %s", path, status, body)
		}
	}
}

func TestReadinessFailsWhileADependencyIsDown(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := gatewayhttp.NewResponder(logger)
	verifier, err := auth.NewVerifier(tokenConfig(), nil)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	accounts := &stubAccounts{}
	router := gatewayhttp.NewRouter(gatewayhttp.RouterOptions{
		BasePath:     basePath,
		Verifier:     verifier,
		MaxBodyBytes: 1 << 20,
		Logger:       logger,
		Ready:        func(context.Context) error { return errs.New(errs.CodeInternal, "database unavailable") },
		Auth:         handler.NewAuth(accounts, responder),
		Users:        handler.NewUsers(accounts, responder),
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	status, _ := requestRaw(t, server, http.MethodGet, "/readyz", "", "")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want 503", status)
	}
	// Liveness 必须保持健康，这样依赖故障才不会触发重启循环。
	if liveStatus, _ := requestRaw(t, server, http.MethodGet, "/healthz", "", ""); liveStatus != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", liveStatus)
	}
}

func TestRegisterRateLimitUsesTheContractEnvelopeAndHeaders(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := gatewayhttp.NewResponder(logger)
	verifier, err := auth.NewVerifier(tokenConfig(), nil)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	limiter, err := middleware.NewRateLimiter(middleware.RateLimitConfig{
		Window: time.Minute, Register: 1, MaxKeys: 10,
	}, responder)
	if err != nil {
		t.Fatalf("NewRateLimiter: %v", err)
	}
	accounts := &stubAccounts{}
	router := gatewayhttp.NewRouter(gatewayhttp.RouterOptions{
		BasePath:     basePath,
		Verifier:     verifier,
		MaxBodyBytes: 1 << 20,
		Logger:       logger,
		Ready:        func(context.Context) error { return nil },
		RateLimiter:  limiter,
		Auth:         handler.NewAuth(accounts, responder),
		Users:        handler.NewUsers(accounts, responder),
	})

	for attempt := 1; attempt <= 2; attempt++ {
		request := httptest.NewRequest(http.MethodPost, basePath+"/auth/register",
			strings.NewReader(`{"student_no":"20269999","password":"correct-horse-battery"}`))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "192.0.2.10:4321"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if attempt == 1 && response.Code != http.StatusCreated {
			t.Fatalf("first status = %d: %s", response.Code, response.Body.String())
		}
		if attempt == 2 {
			if response.Code != http.StatusTooManyRequests {
				t.Fatalf("second status = %d: %s", response.Code, response.Body.String())
			}
			assertErrorCode(t, response.Body.String(), errs.CodeRateLimited)
			if response.Header().Get("Retry-After") == "" || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("rate limit headers = %v", response.Header())
			}
		}
	}
}

// --- 辅助函数 ---------------------------------------------------------------

func request(t *testing.T, server *httptest.Server, method, path, token, body string) (int, string) {
	t.Helper()

	header := ""
	if token != "" {
		header = "Bearer " + token
	}
	return requestRaw(t, server, method, path, header, body)
}

// requestWithHeaders issues a request with extra headers alongside the token.
func requestWithHeaders(t *testing.T, server *httptest.Server, method, path, token, body string, headers map[string]string) (int, string) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(payload)
}

func requestRaw(t *testing.T, server *httptest.Server, method, path, authorization, body string) (int, string) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	return resp.StatusCode, string(payload)
}

func decode(t *testing.T, body string, target any) {
	t.Helper()

	if err := json.Unmarshal([]byte(body), target); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, body)
	}
}

func assertErrorCode(t *testing.T, body string, want errs.Code) {
	t.Helper()

	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	decode(t, body, &envelope)

	if envelope.Code != string(want) {
		t.Fatalf("error code = %q, want %q (%s)", envelope.Code, want, body)
	}
	if envelope.Message == "" {
		t.Fatalf("ErrorResponse requires a non-empty message: %s", body)
	}
}

func foreignToken(t *testing.T) string {
	t.Helper()

	cfg := tokenConfig()
	cfg.Issuer = "somebody-else"
	issuer, err := auth.NewIssuer(cfg, nil, func() string { return "jti" })
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	token, _, err := issuer.Issue(testActor)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return token
}

// --- 下游桩实现 -------------------------------------------------------------

// stubAccounts 代替 Account Service。该服务自身的行为由基于真实数据库的集成测试
// 覆盖；这里的测试关注 Gateway 必须实现的公开 HTTP 契约。
type stubAccounts struct {
	changePasswordErr error
	withoutContact    bool
	lastUpdate        *accountv1.UpdateProfileRequest
	batchCalls        int
	lastBatchSize     int
}

func (s *stubAccounts) userMe() *accountv1.UserMe {
	wechat := "wx_xiaoming"
	return &accountv1.UserMe{
		Id:        testActor,
		StudentNo: "20260001",
		Nickname:  "小明",
		Wechat:    &wechat,
		CreatedAt: timestamppb.New(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)),
		UpdatedAt: timestamppb.New(time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)),
	}
}

func (s *stubAccounts) authData() *accountv1.AuthData {
	return &accountv1.AuthData{
		AccessToken: "signed-token",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
		User:        s.userMe(),
	}
}

func (s *stubAccounts) Register(_ context.Context, _ *accountv1.RegisterRequest, _ ...grpc.CallOption) (*accountv1.RegisterResponse, error) {
	return &accountv1.RegisterResponse{Auth: s.authData()}, nil
}

func (s *stubAccounts) Login(_ context.Context, _ *accountv1.LoginRequest, _ ...grpc.CallOption) (*accountv1.LoginResponse, error) {
	return &accountv1.LoginResponse{Auth: s.authData()}, nil
}

func (s *stubAccounts) GetUser(_ context.Context, req *accountv1.GetUserRequest, _ ...grpc.CallOption) (*accountv1.GetUserResponse, error) {
	user := &accountv1.UserContact{Id: req.GetUserId(), Nickname: "小明"}
	if !s.withoutContact {
		wechat := "wx_xiaoming"
		user.Wechat = &wechat
	}
	return &accountv1.GetUserResponse{User: user}, nil
}

func (s *stubAccounts) GetProfile(_ context.Context, _ *accountv1.GetProfileRequest, _ ...grpc.CallOption) (*accountv1.GetProfileResponse, error) {
	return &accountv1.GetProfileResponse{User: s.userMe()}, nil
}

func (s *stubAccounts) BatchGetUsers(_ context.Context, req *accountv1.BatchGetUsersRequest, _ ...grpc.CallOption) (*accountv1.BatchGetUsersResponse, error) {
	s.batchCalls++
	s.lastBatchSize = len(req.GetUserIds())

	users := make(map[string]*accountv1.UserPublic, len(req.GetUserIds()))
	for _, id := range req.GetUserIds() {
		users[id] = &accountv1.UserPublic{Id: id, Nickname: "小明"}
	}
	return &accountv1.BatchGetUsersResponse{Users: users}, nil
}

func (s *stubAccounts) UpdateProfile(_ context.Context, req *accountv1.UpdateProfileRequest, _ ...grpc.CallOption) (*accountv1.UpdateProfileResponse, error) {
	s.lastUpdate = req
	return &accountv1.UpdateProfileResponse{User: s.userMe()}, nil
}

func (s *stubAccounts) ChangePassword(_ context.Context, _ *accountv1.ChangePasswordRequest, _ ...grpc.CallOption) (*accountv1.ChangePasswordResponse, error) {
	if s.changePasswordErr != nil {
		return nil, s.changePasswordErr
	}
	return &accountv1.ChangePasswordResponse{}, nil
}
