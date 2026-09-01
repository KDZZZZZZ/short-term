package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/account/internal/application"
	"github.com/KDZZZZZZ/short-term/services/account/internal/domain"
)

func TestRegisterIssuesATokenForANewAccount(t *testing.T) {
	t.Parallel()

	svc, deps := newService(t)

	result, err := svc.Register(t.Context(), application.RegisterCommand{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if result.AccessToken == "" {
		t.Fatal("Register returned no access token")
	}
	if result.ExpiresIn() != 3600 {
		t.Fatalf("ExpiresIn = %d, want 3600", result.ExpiresIn())
	}
	if result.Account.StudentNo != "20260001" {
		t.Fatalf("StudentNo = %q", result.Account.StudentNo)
	}
	if result.Account.PasswordHash == "correct-horse-battery" {
		t.Fatal("the password was stored in clear text")
	}
	if _, ok := deps.repo.accounts[result.Account.ID]; !ok {
		t.Fatal("the account was not stored")
	}
}

func TestRegisterDefaultNicknameNeverDisclosesTheStudentNumber(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)

	result, err := svc.Register(t.Context(), application.RegisterCommand{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if result.Account.Nickname != "校园用户" {
		t.Fatalf("default nickname = %q, want 校园用户", result.Account.Nickname)
	}
}

func TestRegisterHonoursAChosenNickname(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)
	nickname := "小明"

	result, err := svc.Register(t.Context(), application.RegisterCommand{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
		Nickname:  &nickname,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if result.Account.Nickname != nickname {
		t.Fatalf("Nickname = %q, want %q", result.Account.Nickname, nickname)
	}
}

func TestRegisterRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	longNickname := strings.Repeat("x", 51)
	badQQ := "12ab"

	tests := []struct {
		name string
		cmd  application.RegisterCommand
		want errs.Code
	}{
		{
			name: "student number too short",
			cmd:  application.RegisterCommand{StudentNo: "1", Password: "correct-horse-battery"},
			want: errs.CodeValidation,
		},
		{
			name: "password too short",
			cmd:  application.RegisterCommand{StudentNo: "20260001", Password: "short"},
			want: errs.CodeValidation,
		},
		{
			name: "nickname too long",
			cmd:  application.RegisterCommand{StudentNo: "20260001", Password: "correct-horse-battery", Nickname: &longNickname},
			want: errs.CodeValidation,
		},
		{
			name: "qq is not numeric",
			cmd:  application.RegisterCommand{StudentNo: "20260001", Password: "correct-horse-battery", QQ: &badQQ},
			want: errs.CodeValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, _ := newService(t)
			_, err := svc.Register(t.Context(), tt.cmd)
			assertCode(t, err, tt.want)
		})
	}
}

func TestRegisterReportsATakenStudentNumber(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)
	cmd := application.RegisterCommand{StudentNo: "20260001", Password: "correct-horse-battery"}

	if _, err := svc.Register(t.Context(), cmd); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	_, err := svc.Register(t.Context(), cmd)
	assertCode(t, err, errs.CodeStudentNoExists)
}

func TestLoginSucceedsWithTheRegisteredPassword(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)
	registered, err := svc.Register(t.Context(), application.RegisterCommand{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := svc.Login(t.Context(), application.LoginCommand{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.Account.ID != registered.Account.ID {
		t.Fatalf("Login returned account %q, want %q", result.Account.ID, registered.Account.ID)
	}
}

func TestLoginDoesNotRevealWhetherAStudentNumberExists(t *testing.T) {
	t.Parallel()

	svc, deps := newService(t)
	if _, err := svc.Register(t.Context(), application.RegisterCommand{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	tests := []struct {
		name string
		cmd  application.LoginCommand
	}{
		{name: "unknown student number", cmd: application.LoginCommand{StudentNo: "20269999", Password: "correct-horse-battery"}},
		{name: "wrong password", cmd: application.LoginCommand{StudentNo: "20260001", Password: "wrong-password-here"}},
		{name: "malformed student number", cmd: application.LoginCommand{StudentNo: "!!", Password: "correct-horse-battery"}},
		{name: "malformed password", cmd: application.LoginCommand{StudentNo: "20260001", Password: "short"}},
	}

	var messages []string
	for _, tt := range tests {
		before := deps.hasher.verifications()
		_, err := svc.Login(t.Context(), tt.cmd)
		assertCode(t, err, errs.CodeUnauthorized)
		messages = append(messages, errs.MessageOf(err))

		// 每条被拒绝的路径仍必须执行一次验证，否则响应时间会告诉攻击者哪些学号存在。
		if got := deps.hasher.verifications() - before; got != 1 {
			t.Fatalf("%s performed %d verifications, want 1", tt.name, got)
		}
	}

	for _, message := range messages[1:] {
		if message != messages[0] {
			t.Fatalf("login failures return distinguishable messages: %q vs %q", message, messages[0])
		}
	}
}

func TestGetUserReportsAMissingAccount(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)

	_, err := svc.GetUser(t.Context(), "u_missing")
	assertCode(t, err, errs.CodeResourceNotFound)
}

func TestBatchGetUsersSkipsMissingAndDuplicateIdentifiers(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)
	first := register(t, svc, "20260001")
	second := register(t, svc, "20260002")

	accounts, err := svc.BatchGetUsers(t.Context(), []string{first, second, first, "u_missing", ""})
	if err != nil {
		t.Fatalf("BatchGetUsers: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("got %d accounts, want 2", len(accounts))
	}
	if _, ok := accounts["u_missing"]; ok {
		t.Fatal("BatchGetUsers invented a missing account")
	}
}

func TestUpdateProfileAppliesOmittedAndSetPatches(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)
	wechat := "wx_xiaoming"
	qq := "123456789"
	nickname := "小明"

	id := registerFull(t, svc, application.RegisterCommand{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
		Nickname:  &nickname,
		Wechat:    &wechat,
		QQ:        &qq,
	})

	updated, err := svc.UpdateProfile(t.Context(), application.UpdateProfileCommand{
		ActorID: id,
		Wechat:  application.Set("wx_updated"),
		QQ:      application.Set("987654321"),
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	if updated.Nickname != nickname {
		t.Fatalf("an absent nickname patch changed the nickname to %q", updated.Nickname)
	}
	if updated.Wechat == nil || *updated.Wechat != "wx_updated" {
		t.Fatalf("Wechat = %v, want wx_updated", updated.Wechat)
	}
	if updated.QQ == nil || *updated.QQ != "987654321" {
		t.Fatalf("QQ = %v, want 987654321", updated.QQ)
	}
}

func TestUpdateProfileRejectsContactRemoval(t *testing.T) {
	t.Parallel()

	svc, deps := newService(t)
	wechat := "wx_xiaoming"
	qq := "123456789"
	id := registerFull(t, svc, application.RegisterCommand{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
		Wechat:    &wechat,
		QQ:        &qq,
	})

	_, err := svc.UpdateProfile(t.Context(), application.UpdateProfileCommand{
		ActorID: id,
		Wechat:  application.Clear(),
	})
	assertCode(t, err, errs.CodeValidation)

	stored := deps.repo.accounts[id]
	if stored.Wechat == nil || *stored.Wechat != wechat || stored.QQ == nil || *stored.QQ != qq {
		t.Fatalf("rejected removal changed stored contacts: wechat=%v qq=%v", stored.Wechat, stored.QQ)
	}
}

func TestUpdateProfileRejectsAnEmptyPatch(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)
	id := register(t, svc, "20260001")

	_, err := svc.UpdateProfile(t.Context(), application.UpdateProfileCommand{ActorID: id})
	assertCode(t, err, errs.CodeValidation)
}

func TestUpdateProfileRequiresAnActor(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)

	_, err := svc.UpdateProfile(t.Context(), application.UpdateProfileCommand{Nickname: ptr("小明")})
	assertCode(t, err, errs.CodeUnauthorized)
}

func TestChangePasswordRequiresTheCurrentPassword(t *testing.T) {
	t.Parallel()

	svc, deps := newService(t)
	id := register(t, svc, "20260001")
	before := deps.repo.accounts[id].PasswordHash

	err := svc.ChangePassword(t.Context(), application.ChangePasswordCommand{
		ActorID:     id,
		OldPassword: "not-the-password",
		NewPassword: "a-brand-new-password",
	})
	assertCode(t, err, errs.CodeUnauthorized)

	if deps.repo.accounts[id].PasswordHash != before {
		t.Fatal("a rejected change still replaced the stored hash")
	}
}

func TestChangePasswordReplacesTheStoredHash(t *testing.T) {
	t.Parallel()

	svc, deps := newService(t)
	id := register(t, svc, "20260001")
	before := deps.repo.accounts[id].PasswordHash

	if err := svc.ChangePassword(t.Context(), application.ChangePasswordCommand{
		ActorID:     id,
		OldPassword: "correct-horse-battery",
		NewPassword: "a-brand-new-password",
	}); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	after := deps.repo.accounts[id].PasswordHash
	if after == before {
		t.Fatal("the stored hash did not change")
	}
	if strings.Contains(after, "a-brand-new-password") {
		t.Fatal("the new password was stored in clear text")
	}

	if _, err := svc.Login(t.Context(), application.LoginCommand{
		StudentNo: "20260001",
		Password:  "a-brand-new-password",
	}); err != nil {
		t.Fatalf("login with the new password: %v", err)
	}
	_, err := svc.Login(t.Context(), application.LoginCommand{
		StudentNo: "20260001",
		Password:  "correct-horse-battery",
	})
	assertCode(t, err, errs.CodeUnauthorized)
}

func TestChangePasswordValidatesTheNewPassword(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)
	id := register(t, svc, "20260001")

	err := svc.ChangePassword(t.Context(), application.ChangePasswordCommand{
		ActorID:     id,
		OldPassword: "correct-horse-battery",
		NewPassword: "short",
	})
	assertCode(t, err, errs.CodeValidation)
}

func TestErrorsNeverEchoTheSubmittedPassword(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)
	const secret = "super-secret-password"

	_, registerErr := svc.Register(t.Context(), application.RegisterCommand{StudentNo: "!", Password: secret})
	_, loginErr := svc.Login(t.Context(), application.LoginCommand{StudentNo: "20260001", Password: secret})

	for _, err := range []error{registerErr, loginErr} {
		if err == nil {
			t.Fatal("expected an error")
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error text leaked the password: %v", err)
		}
	}
}

// --- 测试替身 ---------------------------------------------------------------

type deps struct {
	repo   *fakeRepository
	hasher *fakeHasher
}

func newService(t *testing.T) (*application.Service, *deps) {
	t.Helper()

	d := &deps{repo: newFakeRepository(), hasher: &fakeHasher{}}
	svc, err := application.NewService(
		d.repo,
		d.hasher,
		&fakeIssuer{ttl: time.Hour},
		&fakeIDs{},
		fixedClock{at: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, d
}

func register(t *testing.T, svc *application.Service, studentNo string) string {
	t.Helper()

	return registerFull(t, svc, application.RegisterCommand{StudentNo: studentNo, Password: "correct-horse-battery"})
}

func registerFull(t *testing.T, svc *application.Service, cmd application.RegisterCommand) string {
	t.Helper()

	result, err := svc.Register(t.Context(), cmd)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return result.Account.ID
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

func ptr(value string) *string { return &value }

type fakeRepository struct {
	mu        sync.Mutex
	accounts  map[string]*domain.Account
	byStudent map[string]string
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{accounts: map[string]*domain.Account{}, byStudent: map[string]string{}}
}

func (r *fakeRepository) Create(_ context.Context, account *domain.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, taken := r.byStudent[account.StudentNo]; taken {
		return application.ErrStudentNoTaken
	}
	stored := *account
	r.accounts[account.ID] = &stored
	r.byStudent[account.StudentNo] = account.ID
	return nil
}

func (r *fakeRepository) ByID(_ context.Context, id string) (*domain.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, ok := r.accounts[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	copied := *account
	return &copied, nil
}

func (r *fakeRepository) ByStudentNo(ctx context.Context, studentNo string) (*domain.Account, error) {
	r.mu.Lock()
	id, ok := r.byStudent[studentNo]
	r.mu.Unlock()
	if !ok {
		return nil, application.ErrNotFound
	}
	return r.ByID(ctx, id)
}

func (r *fakeRepository) ByIDs(ctx context.Context, ids []string) ([]*domain.Account, error) {
	accounts := make([]*domain.Account, 0, len(ids))
	for _, id := range ids {
		account, err := r.ByID(ctx, id)
		if errors.Is(err, application.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

func (r *fakeRepository) Update(_ context.Context, account *domain.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.accounts[account.ID]; !ok {
		return application.ErrNotFound
	}
	stored := *account
	r.accounts[account.ID] = &stored
	return nil
}

// fakeHasher 是可逆的替身：真实 Argon2id 实现在 platform/auth 中测试，
// 在这里运行会让测试变慢，却不会增加新的测试内容。
type fakeHasher struct {
	mu      sync.Mutex
	verifed int
}

func (h *fakeHasher) Hash(password string) (string, error) { return fakeDigest(password), nil }

func (h *fakeHasher) Verify(password, encoded string) error {
	h.mu.Lock()
	h.verifed++
	h.mu.Unlock()

	if encoded != fakeDigest(password) {
		return errors.New("mismatch")
	}
	return nil
}

func (h *fakeHasher) NeedsRehash(string) bool { return false }

// fakeDigest 和真实哈希器一样是单向的，因此使用该替身时，断言明文密码不会进入
// 存储仍然有意义。
func fakeDigest(password string) string {
	sum := sha256.Sum256([]byte("fake-hash:" + password))
	return "fake$" + hex.EncodeToString(sum[:])
}

func (h *fakeHasher) verifications() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.verifed
}

type fakeIssuer struct {
	ttl time.Duration
}

func (i *fakeIssuer) Issue(subject string) (string, time.Time, error) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	return "token-for-" + subject, now.Add(i.ttl), nil
}

type fakeIDs struct {
	mu sync.Mutex
	n  int
}

func (g *fakeIDs) New() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return "u_" + strings.Repeat("0", 3) + string(rune('a'+g.n))
}

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }
