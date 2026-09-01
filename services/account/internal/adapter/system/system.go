// Package system 将进程级设施——时钟和标识生成器——适配到 Account Service 应用端口。
package system

import (
	"time"

	"github.com/KDZZZZZZ/short-term/platform/id"
)

// Clock 读取进程时钟。
type Clock struct{}

// Now 返回当前 UTC 时间，并截断到微秒。
//
// 存储 UTC 可以让时间戳不受主机时区影响而保持可比较；截断到 PostgreSQL 支持的精度，
// 意味着调用方从写入操作得到的值与稍后读取的值逐字节相同。
func (Clock) Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// IDs 生成账户标识。
type IDs struct {
	generator *id.Generator
}

// NewIDs 构造账户标识生成器。
func NewIDs() *IDs { return &IDs{generator: id.NewGenerator(nil)} }

// New 返回新的不透明账户标识。
func (i *IDs) New() string { return i.generator.New(id.PrefixAccount) }
