package application

import (
	"context"
	"errors"
	"log/slog"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/observability"
	"github.com/KDZZZZZZ/short-term/services/account/internal/domain"
)

// defaultNicknamePrefix starts the nickname assigned when a student registers
// without choosing one.
//
// Agent Self-Claimed: the default must not be derived from the student number.
// The approved contract forbids disclosing a seller's student number
// (openapi/paths/products.yaml, getProduct), and nicknames are shown on every
// product, conversation and trade projection, so a student-number-derived
// nickname would leak it through the front door.
const defaultNicknamePrefix = "同学"

// Service implements the Account Service use cases.
type Service struct {
	repo    Repository
	hasher  PasswordHasher
	tokens  TokenIssuer
	ids     IDGenerator
	clock   Clock
	logger  *slog.Logger
	decoyPW string
}

// NewService wires the use cases. decoyHash is a hash of an unguessable value;
// Login verifies against it when no account matches so that a missing account
// and a wrong password cost the same time and cannot be told apart.
func NewService(repo Repository, hasher PasswordHasher, tokens TokenIssuer, ids IDGenerator, clock Clock, logger *slog.Logger) (*Service, error) {
	if repo == nil || hasher == nil || tokens == nil || ids == nil || clock == nil {
		return nil, errors.New("application: every Account Service dependency is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	decoy, err := hasher.Hash(ids.New())
	if err != nil {
		return nil, err
	}

	return &Service{repo: repo, hasher: hasher, tokens: tokens, ids: ids, clock: clock, logger: logger, decoyPW: decoy}, nil
}

// Register creates an account and returns an access token for it.
func (s *Service) Register(ctx context.Context, cmd RegisterCommand) (AuthResult, error) {
	if err := domain.ValidateStudentNo(cmd.StudentNo); err != nil {
		return AuthResult{}, errs.Wrap(errs.CodeValidation, "学号格式不合法", err)
	}
	if err := domain.ValidatePassword(cmd.Password); err != nil {
		return AuthResult{}, errs.Wrap(errs.CodeValidation, "密码长度必须为 8 至 64 个字符", err)
	}

	accountID := s.ids.New()
	nickname := defaultNickname(accountID)
	if cmd.Nickname != nil {
		nickname = *cmd.Nickname
	}

	hash, err := s.hasher.Hash(cmd.Password)
	if err != nil {
		return AuthResult{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}

	now := s.clock.Now()
	account, err := domain.New(accountID, cmd.StudentNo, hash, nickname, cmd.Wechat, cmd.QQ, now)
	if err != nil {
		return AuthResult{}, errs.Wrap(errs.CodeValidation, "注册信息不合法", err)
	}

	if err := s.repo.Create(ctx, account); err != nil {
		if errors.Is(err, ErrStudentNoTaken) {
			return AuthResult{}, errs.Wrap(errs.CodeStudentNoExists, "该学号已注册", err)
		}
		return AuthResult{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}

	return s.issue(account)
}

// Login authenticates a student number and password pair.
func (s *Service) Login(ctx context.Context, cmd LoginCommand) (AuthResult, error) {
	// Format problems are reported as a failed login rather than as a
	// validation error: telling a caller which student numbers are well formed
	// is free reconnaissance.
	if domain.ValidateStudentNo(cmd.StudentNo) != nil || domain.ValidatePassword(cmd.Password) != nil {
		s.burnTime(cmd.Password)
		return AuthResult{}, errs.New(errs.CodeUnauthorized, "学号或密码错误")
	}

	account, err := s.repo.ByStudentNo(ctx, cmd.StudentNo)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.burnTime(cmd.Password)
			return AuthResult{}, errs.New(errs.CodeUnauthorized, "学号或密码错误")
		}
		return AuthResult{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}

	if err := s.hasher.Verify(cmd.Password, account.PasswordHash); err != nil {
		return AuthResult{}, errs.New(errs.CodeUnauthorized, "学号或密码错误")
	}

	s.upgradeHashIfNeeded(ctx, account, cmd.Password)

	return s.issue(account)
}

// GetUser returns one account with its contact details. Contact details are
// part of the approved product detail response, so this RPC is only called by
// the Gateway on paths that are allowed to show them.
func (s *Service) GetUser(ctx context.Context, userID string) (*domain.Account, error) {
	if userID == "" {
		return nil, errs.New(errs.CodeValidation, "用户标识不能为空")
	}

	account, err := s.repo.ByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, errs.Wrap(errs.CodeResourceNotFound, "用户不存在", err)
		}
		return nil, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return account, nil
}

// BatchGetUsers returns the accounts that exist among ids. It exists so list
// endpoints can complete seller and participant names in one round trip
// instead of one call per row (docs/software-design.md section 3.3).
func (s *Service) BatchGetUsers(ctx context.Context, ids []string) (map[string]*domain.Account, error) {
	unique := dedupe(ids)
	if len(unique) == 0 {
		return map[string]*domain.Account{}, nil
	}

	accounts, err := s.repo.ByIDs(ctx, unique)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}

	byID := make(map[string]*domain.Account, len(accounts))
	for _, account := range accounts {
		byID[account.ID] = account
	}
	return byID, nil
}

// UpdateProfile changes the caller's own nickname and contact details.
func (s *Service) UpdateProfile(ctx context.Context, cmd UpdateProfileCommand) (*domain.Account, error) {
	if cmd.ActorID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if cmd.Nickname == nil && !cmd.Wechat.Present && !cmd.QQ.Present {
		return nil, errs.New(errs.CodeValidation, "请至少修改一项资料")
	}

	account, err := s.GetUser(ctx, cmd.ActorID)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	if cmd.Nickname != nil {
		if err := account.Rename(*cmd.Nickname, now); err != nil {
			return nil, errs.Wrap(errs.CodeValidation, "昵称长度必须为 1 至 50 个字符", err)
		}
	}
	if cmd.Wechat.Present {
		if err := account.SetWechat(cmd.Wechat.Value, now); err != nil {
			return nil, errs.Wrap(errs.CodeValidation, "微信号长度必须为 1 至 64 个字符", err)
		}
	}
	if cmd.QQ.Present {
		if err := account.SetQQ(cmd.QQ.Value, now); err != nil {
			return nil, errs.Wrap(errs.CodeValidation, "QQ 号必须为 5 至 20 位数字", err)
		}
	}

	if err := s.repo.Update(ctx, account); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, errs.Wrap(errs.CodeResourceNotFound, "用户不存在", err)
		}
		return nil, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return account, nil
}

// ChangePassword replaces the caller's own password after checking the old one.
func (s *Service) ChangePassword(ctx context.Context, cmd ChangePasswordCommand) error {
	if cmd.ActorID == "" {
		return errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if err := domain.ValidatePassword(cmd.NewPassword); err != nil {
		return errs.Wrap(errs.CodeValidation, "新密码长度必须为 8 至 64 个字符", err)
	}

	account, err := s.GetUser(ctx, cmd.ActorID)
	if err != nil {
		return err
	}

	// A wrong current password is an authentication failure, not a validation
	// error: the caller holds a valid token but has not proven the password.
	if err := s.hasher.Verify(cmd.OldPassword, account.PasswordHash); err != nil {
		return errs.New(errs.CodeUnauthorized, "当前密码错误")
	}

	hash, err := s.hasher.Hash(cmd.NewPassword)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	if err := account.SetPasswordHash(hash, s.clock.Now()); err != nil {
		return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	if err := s.repo.Update(ctx, account); err != nil {
		return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return nil
}

// issue signs an access token for an authenticated account.
func (s *Service) issue(account *domain.Account) (AuthResult, error) {
	token, expiresAt, err := s.tokens.Issue(account.ID)
	if err != nil {
		return AuthResult{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return AuthResult{AccessToken: token, IssuedAt: s.clock.Now(), ExpiresAt: expiresAt, Account: account}, nil
}

// burnTime performs the same work a real verification would, so an attacker
// cannot distinguish an unknown student number from a wrong password by
// measuring response time.
func (s *Service) burnTime(password string) {
	_ = s.hasher.Verify(password, s.decoyPW)
}

// upgradeHashIfNeeded re-hashes a password that was stored with weaker
// parameters than the current configuration. A failure here must not fail the
// login: the caller already proved the password.
func (s *Service) upgradeHashIfNeeded(ctx context.Context, account *domain.Account, password string) {
	if !s.hasher.NeedsRehash(account.PasswordHash) {
		return
	}

	logger := observability.LoggerWith(ctx, s.logger)
	hash, err := s.hasher.Hash(password)
	if err != nil {
		logger.Warn("password rehash failed", slog.String("error", err.Error()))
		return
	}
	if err := account.SetPasswordHash(hash, s.clock.Now()); err != nil {
		logger.Warn("password rehash rejected", slog.String("error", err.Error()))
		return
	}
	if err := s.repo.Update(ctx, account); err != nil {
		logger.Warn("password rehash not persisted", slog.String("error", err.Error()))
	}
}

// defaultNickname derives a neutral display name from the opaque account
// identifier. The identifier is already public, so this discloses nothing new.
func defaultNickname(accountID string) string {
	suffix := accountID
	if len(suffix) > 4 {
		suffix = suffix[len(suffix)-4:]
	}
	return defaultNicknamePrefix + suffix
}

// dedupe removes empty and repeated identifiers while preserving order.
func dedupe(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	unique := make([]string, 0, len(ids))
	for _, value := range ids {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
