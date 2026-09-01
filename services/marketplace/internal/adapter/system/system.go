// Package system 将进程设施适配到 Marketplace 应用端口。
package system

import (
	"time"

	"github.com/KDZZZZZZ/short-term/platform/id"
)

// Clock 读取进程时钟。
type Clock struct{}

// Now 返回当前 UTC 时间，并截断到 PostgreSQL 存储的精度，
// 使写入返回的值与稍后读取的值完全一致。
func (Clock) Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// IDs 生成 Marketplace 标识。
type IDs struct {
	generator *id.Generator
}

// NewIDs 构造标识生成器。
func NewIDs() *IDs { return &IDs{generator: id.NewGenerator(nil)} }

// NewProductID 返回新的不透明商品标识。
func (i *IDs) NewProductID() string { return i.generator.New(id.PrefixProduct) }

// NewImageID 返回新的不透明图片标识。
func (i *IDs) NewImageID() string { return i.generator.New(id.PrefixProductImage) }

// NewTradeID 返回新的不透明交易标识。
func (i *IDs) NewTradeID() string { return i.generator.New(id.PrefixTrade) }

// NewEventID 返回新的不透明 Outbox 事件标识。
func (i *IDs) NewEventID() string { return i.generator.New(id.PrefixEvent) }
