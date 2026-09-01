// Package application 保存 Marketplace 用例：商品发布与浏览、图片以及交易状态机。
package application

import (
	"context"
	"errors"
	"time"

	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// 存储结果。它们描述仓储结果，而不是 HTTP 或 gRPC 结果。
var (
	// ErrNotFound 表示没有匹配的行。
	ErrNotFound = errors.New("not found")
	// ErrVersionConflict 表示该行在读取后发生了变化。
	ErrVersionConflict = errors.New("product changed concurrently")
	// ErrImageSlotTaken 表示其他写入方先占用了图片位置。
	ErrImageSlotTaken = errors.New("image slot already used")
)

// ProductFilter 为公开列表筛选商品。
type ProductFilter struct {
	// Keyword 匹配标题子串。nil 表示不按关键词筛选。
	Keyword *string
	// Category 限定分类。nil 表示所有分类。
	Category *domain.Category
	// Status 限定状态。nil 表示所有状态；公开列表始终将其设置为 ON_SALE。
	Status *domain.Status
	// SellerID 限定一个卖家。为空表示所有卖家。
	SellerID string
}

// Page 描述分页请求。仓储使用确定性顺序，并以标识作为平局裁决，
// 因此分页不会跳过或重复时间戳相同的行。
type Page struct {
	Number int32
	Size   int32
}

// ProductPage 是一页结果及总行数。
type ProductPage struct {
	Items []*domain.Product
	Page  int32
	Size  int32
	Total int64
}

// ProductMutation 在已锁定 Product 行并读取当前 PENDING 意向事实后执行。
// 返回错误会回滚商品字段、图片和顺序的全部数据库变化。
type ProductMutation func(product *domain.Product, hasPendingIntent bool) error

// ProductRepository 存储商品及其图片。
type ProductRepository interface {
	Create(ctx context.Context, product *domain.Product) error
	// ByID 加载一个商品及其图片。
	ByID(ctx context.Context, id string) (*domain.Product, error)
	// ByIDs 在一次往返中加载多个商品及其图片。
	ByIDs(ctx context.Context, ids []string) ([]*domain.Product, error)
	List(ctx context.Context, filter ProductFilter, page Page) (ProductPage, error)
	// Mutate 与购买意向创建共用 Product 行锁，并在同一事务中持久化字段及图片差异。
	// 这样不能提交 OFF_SHELF + PENDING，也不会让价格快照与商品编辑交错。
	Mutate(ctx context.Context, productID string, mutation ProductMutation) (*domain.Product, error)
}

// ObjectStore 保存商品图片。
//
// 该接口有意保持精简，使部署实现可以使用 Alibaba Cloud OSS
// （docs/software-design.md 第 9.2 节），而本地开发和测试使用文件系统。
// 调用方无需知道当前使用哪种实现。
type ObjectStore interface {
	// Put 将数据保存到 key 下。实现必须覆盖已有对象，使重试上传具备幂等性。
	Put(ctx context.Context, key, contentType string, data []byte) error
	// Delete 删除对象。删除不存在的对象不算错误。
	Delete(ctx context.Context, key string) error
	// URL 返回对象可公开访问的位置。
	URL(key string) string
}

// IDGenerator 生成 Marketplace 聚合的不透明标识。
type IDGenerator interface {
	NewProductID() string
	NewImageID() string
	NewTradeID() string
	NewEventID() string
}

// Clock 读取当前时间。
type Clock interface {
	Now() time.Time
}
