// Package config 从进程环境变量加载服务配置。
//
// 密钥由运行时（GitHub Actions 密钥或服务器环境）注入，绝不提交到仓库，
// 因此环境变量是唯一支持的来源。Loader 会收集所有问题并一次性报告，
// 使错误配置的部署通过一条完整消息快速失败，而不是每次只报告一个变量。
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Loader 从环境变量读取类型化的值，并收集发现的所有问题。
type Loader struct {
	lookup func(string) (string, bool)
	errs   []error
}

// NewLoader 基于 os.LookupEnv 构造 Loader。
func NewLoader() *Loader { return &Loader{lookup: os.LookupEnv} }

// NewLoaderFrom 基于显式环境构造 Loader，供测试使用。
func NewLoaderFrom(env map[string]string) *Loader {
	return &Loader{lookup: func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}}
}

// Err 返回合并后的配置错误；全部加载成功时返回 nil。
func (l *Loader) Err() error { return errors.Join(l.errs...) }

// String 在变量未设置或为空时返回备用值，否则返回变量值。
func (l *Loader) String(key, fallback string) string {
	value, ok := l.lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// Required 返回变量；变量缺失时记录错误。
func (l *Loader) Required(key string) string {
	value, ok := l.lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		l.errs = append(l.errs, fmt.Errorf("config: %s is required", key))
		return ""
	}
	return value
}

// Int 返回整数变量；变量缺失或为空时返回备用值。
func (l *Loader) Int(key string, fallback int) int {
	raw, ok := l.lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("config: %s must be an integer: %w", key, err))
		return fallback
	}
	return value
}

// Duration 返回时长变量或备用值，并接受 Go 语法，例如 "250ms" 或 "5s"。
func (l *Loader) Duration(key string, fallback time.Duration) time.Duration {
	raw, ok := l.lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("config: %s must be a duration: %w", key, err))
		return fallback
	}
	return value
}

// Bool 返回布尔变量或备用值。
func (l *Loader) Bool(key string, fallback bool) bool {
	raw, ok := l.lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("config: %s must be a boolean: %w", key, err))
		return fallback
	}
	return value
}

// Fail 记录加载后发现的校验问题，使调用方可以通过一个渠道报告配置错误和
// 跨字段错误。
func (l *Loader) Fail(format string, args ...any) {
	l.errs = append(l.errs, fmt.Errorf(format, args...))
}
