package application

import (
	"context"
	"errors"
	"log/slog"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/observability"
	"github.com/KDZZZZZZ/short-term/services/account/internal/domain"
)

// defaultNickname 是 OpenAPI 对省略昵称注册请求规定的固定默认值。
const defaultNickname = "校园用户"

// Service 实现 Account Service 用例。
type Service struct {
	repo    Repository
	hasher  PasswordHasher
	tokens  TokenIssuer
	ids     IDGenerator
	clock   Clock
	logger  *slog.Logger
	decoyPW string
}

// NewService 组装各个用例。decoyHash 是不可猜值的哈希；
// 没有匹配账户时，Login 会用它执行验证，使账户不存在和密码错误消耗相同时间，
// 从而无法区分二者。
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

// Register 创建账户并返回该账户的访问令牌。
func (s *Service) Register(ctx context.Context, cmd RegisterCommand) (AuthResult, error) {
	if err := domain.ValidateStudentNo(cmd.StudentNo); err != nil {
		return AuthResult{}, errs.Wrap(errs.CodeValidation, "学号格式不合法", err)
	}
	if err := domain.ValidatePassword(cmd.Password); err != nil {
		return AuthResult{}, errs.Wrap(errs.CodeValidation, "密码长度必须为 8 至 64 个字符", err)
	}

	accountID := s.ids.New()
	nickname := defaultNickname
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

// Login 验证学号和密码组合。
func (s *Service) Login(ctx context.Context, cmd LoginCommand) (AuthResult, error) {
	// 格式问题报告为登录失败，而不是校验错误：告诉调用方哪些学号格式正确，
	// 等于免费提供侦察信息。
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

// GetUser 返回一个包含联系方式的账户。联系方式是已批准商品详情响应的一部分，
// 因此只有允许展示联系方式的路径才会由 Gateway 调用此 RPC。
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

// BatchGetUsers 返回指定 id 中存在的账户。它让列表端点可以在一次往返中补全卖家和
// 参与者名称，而不是每行调用一次（docs/software-design.md 第 3.3 节）。
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

// UpdateProfile 修改调用方自己的昵称和联系方式。
func (s *Service) UpdateProfile(ctx context.Context, cmd UpdateProfileCommand) (*domain.Account, error) {
	if cmd.ActorID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if cmd.Nickname == nil && !cmd.Wechat.Present && !cmd.QQ.Present {
		return nil, errs.New(errs.CodeValidation, "请至少修改一项资料")
	}
	if (cmd.Wechat.Present && cmd.Wechat.Value == nil) || (cmd.QQ.Present && cmd.QQ.Value == nil) {
		return nil, errs.New(errs.CodeValidation, "微信和 QQ 只能新增或修改，不能删除")
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
			return nil, errs.Wrap(errs.CodeValidation, "微信号必须为 1 至 64 个非空白字符", err)
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

// ChangePassword 校验旧密码后替换调用方自己的密码。
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

	// 当前密码错误属于身份认证失败，而不是校验错误：调用方虽然持有有效令牌，
	// 但尚未证明自己知道密码。
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

// issue 为通过认证的账户签发访问令牌。
func (s *Service) issue(account *domain.Account) (AuthResult, error) {
	token, expiresAt, err := s.tokens.Issue(account.ID)
	if err != nil {
		return AuthResult{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return AuthResult{AccessToken: token, IssuedAt: s.clock.Now(), ExpiresAt: expiresAt, Account: account}, nil
}

// burnTime 执行与真实验证相同的工作，使攻击者无法通过测量响应时间区分未知学号
// 和错误密码。
func (s *Service) burnTime(password string) {
	_ = s.hasher.Verify(password, s.decoyPW)
}

// upgradeHashIfNeeded 重新哈希使用弱于当前配置参数存储的密码。
// 这里失败不能导致登录失败：调用方已经证明了密码正确。
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

// dedupe 删除空标识和重复标识，同时保留原有顺序。
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
