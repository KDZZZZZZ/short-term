// Package postgres 基于 Marketplace Service 自有数据库实现仓储端口。
package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// imageSlotConstraint 是串行化争抢同一图片位置的两个写入方的唯一索引。
const imageSlotConstraint = "product_images_slot_unique"

// productColumns 是所有商品读取共享的字段投影。
const productColumns = `id, seller_id, title, price_minor, category, description, status, version, created_at, updated_at`

// ProductRepository 在 PostgreSQL 中存储商品及其图片。
type ProductRepository struct {
	pool *pgxpool.Pool
}

// NewProductRepository 基于已打开的连接池构造仓储。
func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

var _ application.ProductRepository = (*ProductRepository)(nil)

// Create 在一条事务中插入商品及其初始图片，因此列表永远不会看到缺少发布时图片的商品。
func (r *ProductRepository) Create(ctx context.Context, product *domain.Product) error {
	return pg.InTx(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		const insertProduct = `
			INSERT INTO products (` + productColumns + `)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

		_, err := tx.Exec(ctx, insertProduct,
			product.ID, product.SellerID, product.Title, product.PriceMinor,
			string(product.Category), product.Description, string(product.Status),
			product.Version, product.CreatedAt, product.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("postgres: insert product: %w", err)
		}

		for i := range product.Images {
			if err := insertImage(ctx, tx, &product.Images[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// ByID 加载一个商品及其图片。
func (r *ProductRepository) ByID(ctx context.Context, id string) (*domain.Product, error) {
	const query = `SELECT ` + productColumns + ` FROM products WHERE id = $1`

	product, err := scanProduct(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		return nil, fmt.Errorf("postgres: select product: %w", err)
	}

	images, err := r.imagesFor(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	product.Images = images[id]
	return product, nil
}

// ByIDs 通过两次往返加载多个商品及其图片。
func (r *ProductRepository) ByIDs(ctx context.Context, ids []string) ([]*domain.Product, error) {
	const query = `SELECT ` + productColumns + ` FROM products WHERE id = ANY($1)`

	rows, err := r.pool.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("postgres: select products: %w", err)
	}
	products, err := collectProducts(rows)
	if err != nil {
		return nil, err
	}

	return r.attachImages(ctx, products)
}

// List 按确定性顺序返回一页商品。
func (r *ProductRepository) List(ctx context.Context, filter application.ProductFilter, page application.Page) (application.ProductPage, error) {
	where, args := buildFilter(filter)

	countQuery := `SELECT count(*) FROM products` + where
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return application.ProductPage{}, fmt.Errorf("postgres: count products: %w", err)
	}

	// created_at 单独并不唯一，因此使用 id 作为平局裁决。没有它，分页边界可能会重复
	// 或跳过在同一时刻发布的商品（docs/software-design.md 第 8.2 节）。
	listQuery := fmt.Sprintf(
		`SELECT %s FROM products%s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d`,
		productColumns, where, len(args)+1, len(args)+2,
	)
	args = append(args, page.Size, page.Offset())

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return application.ProductPage{}, fmt.Errorf("postgres: select products: %w", err)
	}
	products, err := collectProducts(rows)
	if err != nil {
		return application.ProductPage{}, err
	}
	products, err = r.attachImages(ctx, products)
	if err != nil {
		return application.ProductPage{}, err
	}

	return application.ProductPage{
		Items: products,
		Page:  page.Number,
		Size:  page.Size,
		Total: total,
	}, nil
}

// Mutate serializes product changes with purchase-intent creation by locking
// the Product row first. It then observes PENDING intents and persists product
// fields plus the complete image-order diff in the same transaction.
func (r *ProductRepository) Mutate(ctx context.Context, productID string, mutation application.ProductMutation) (*domain.Product, error) {
	var result *domain.Product
	err := pg.InTx(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		const lockProduct = `SELECT ` + productColumns + ` FROM products WHERE id = $1 FOR UPDATE`
		product, err := scanProduct(tx.QueryRow(ctx, lockProduct, productID))
		if err != nil {
			return fmt.Errorf("postgres: lock product mutation: %w", err)
		}

		images, err := lockedImages(ctx, tx, productID)
		if err != nil {
			return err
		}
		product.Images = images
		original := make(map[string]domain.Image, len(images))
		for _, image := range images {
			original[image.ID] = image
		}

		var hasPending bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM trades WHERE product_id = $1 AND status = 'PENDING')`,
			productID,
		).Scan(&hasPending); err != nil {
			return fmt.Errorf("postgres: inspect pending intents: %w", err)
		}

		expectedVersion := product.Version
		if err := mutation(product, hasPending); err != nil {
			return err
		}

		const updateProduct = `
			UPDATE products
			   SET title = $3, price_minor = $4, category = $5, description = $6,
			       status = $7, version = $8, updated_at = $9
			 WHERE id = $1 AND version = $2`
		tag, err := tx.Exec(ctx, updateProduct,
			product.ID, expectedVersion, product.Title, product.PriceMinor,
			string(product.Category), product.Description, string(product.Status),
			product.Version, product.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("postgres: persist product mutation: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return application.ErrVersionConflict
		}

		current := make(map[string]domain.Image, len(product.Images))
		for _, image := range product.Images {
			current[image.ID] = image
		}
		// Delete removed rows first, which frees their unique sort slots before
		// the remaining images are compacted toward the front.
		for id := range original {
			if _, ok := current[id]; ok {
				continue
			}
			if _, err := tx.Exec(ctx,
				`DELETE FROM product_images WHERE product_id = $1 AND id = $2`, productID, id,
			); err != nil {
				return fmt.Errorf("postgres: delete product image in mutation: %w", err)
			}
		}
		for i := range product.Images {
			image := &product.Images[i]
			if _, existed := original[image.ID]; !existed {
				if err := insertImage(ctx, tx, image); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.Exec(ctx,
				`UPDATE product_images SET sort_order = $3 WHERE product_id = $1 AND id = $2`,
				productID, image.ID, image.SortOrder,
			); err != nil {
				return fmt.Errorf("postgres: reorder product image: %w", err)
			}
		}

		result = product
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func lockedImages(ctx context.Context, tx pgx.Tx, productID string) ([]domain.Image, error) {
	const query = `
		SELECT id, product_id, object_key, sort_order, created_at
		  FROM product_images
		 WHERE product_id = $1
		 ORDER BY sort_order
		 FOR UPDATE`

	rows, err := tx.Query(ctx, query, productID)
	if err != nil {
		return nil, fmt.Errorf("postgres: lock product images: %w", err)
	}
	defer rows.Close()

	var images []domain.Image
	for rows.Next() {
		var image domain.Image
		if err := rows.Scan(&image.ID, &image.ProductID, &image.ObjectKey, &image.SortOrder, &image.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan locked product image: %w", err)
		}
		images = append(images, image)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: read locked product images: %w", err)
	}
	return images, nil
}

// Update 仅在存储版本仍与读取时一致的情况下回写商品，以此检测丢失更新。
func (r *ProductRepository) Update(ctx context.Context, product *domain.Product, expectedVersion int64) error {
	const query = `
		UPDATE products
		   SET title = $3, price_minor = $4, category = $5, description = $6,
		       status = $7, version = $8, updated_at = $9
		 WHERE id = $1 AND version = $2`

	tag, err := r.pool.Exec(ctx, query,
		product.ID, expectedVersion, product.Title, product.PriceMinor,
		string(product.Category), product.Description, string(product.Status),
		product.Version, product.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: update product: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return r.explainMissingUpdate(ctx, product.ID)
	}
	return nil
}

// AddImage 插入一张图片。
func (r *ProductRepository) AddImage(ctx context.Context, image *domain.Image) error {
	return insertImage(ctx, r.pool, image)
}

// DeleteImage 删除一张图片，并返回其对象键。
func (r *ProductRepository) DeleteImage(ctx context.Context, productID, imageID string) (string, error) {
	const query = `DELETE FROM product_images WHERE product_id = $1 AND id = $2 RETURNING object_key`

	var objectKey string
	err := r.pool.QueryRow(ctx, query, productID, imageID).Scan(&objectKey)
	if pg.IsNoRows(err) {
		return "", application.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("postgres: delete product image: %w", err)
	}
	return objectKey, nil
}

// explainMissingUpdate 区分商品已不存在和其他写入方已先行修改商品这两种情况。
func (r *ProductRepository) explainMissingUpdate(ctx context.Context, productID string) error {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT true FROM products WHERE id = $1`, productID).Scan(&exists)
	if pg.IsNoRows(err) {
		return application.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("postgres: inspect product: %w", err)
	}
	return application.ErrVersionConflict
}

// attachImages 通过一次查询加载每个商品的图片。
func (r *ProductRepository) attachImages(ctx context.Context, products []*domain.Product) ([]*domain.Product, error) {
	if len(products) == 0 {
		return products, nil
	}

	ids := make([]string, len(products))
	for i, product := range products {
		ids[i] = product.ID
	}
	images, err := r.imagesFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, product := range products {
		product.Images = images[product.ID]
	}
	return products, nil
}

// imagesFor 一次加载多个商品的图片，避免列表响应中每行执行一次查询。
func (r *ProductRepository) imagesFor(ctx context.Context, productIDs []string) (map[string][]domain.Image, error) {
	const query = `
		SELECT id, product_id, object_key, sort_order, created_at
		  FROM product_images
		 WHERE product_id = ANY($1)
		 ORDER BY product_id, sort_order`

	rows, err := r.pool.Query(ctx, query, productIDs)
	if err != nil {
		return nil, fmt.Errorf("postgres: select product images: %w", err)
	}
	defer rows.Close()

	byProduct := make(map[string][]domain.Image, len(productIDs))
	for rows.Next() {
		var image domain.Image
		if err := rows.Scan(&image.ID, &image.ProductID, &image.ObjectKey, &image.SortOrder, &image.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan product image: %w", err)
		}
		byProduct[image.ProductID] = append(byProduct[image.ProductID], image)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: read product images: %w", err)
	}
	return byProduct, nil
}

// buildFilter 将筛选条件转换为 WHERE 子句及其参数。
func buildFilter(filter application.ProductFilter) (string, []any) {
	var conditions []string
	var args []any

	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if len(filter.Statuses) > 0 {
		values := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			values = append(values, string(status))
		}
		args = append(args, values)
		conditions = append(conditions, fmt.Sprintf("status = ANY($%d)", len(args)))
	}
	if filter.Category != nil {
		args = append(args, string(*filter.Category))
		conditions = append(conditions, fmt.Sprintf("category = $%d", len(args)))
	}
	if filter.SellerID != "" {
		args = append(args, filter.SellerID)
		conditions = append(conditions, fmt.Sprintf("seller_id = $%d", len(args)))
	}
	if filter.Keyword != nil {
		// 关键词是字面子串，因此用户输入中的 LIKE 元字符会被转义，而不是被解释为模式。
		args = append(args, "%"+escapeLike(*filter.Keyword)+"%")
		conditions = append(conditions, fmt.Sprintf(`title ILIKE $%d ESCAPE '\'`, len(args)))
	}

	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

// escapeLike 中和 LIKE 通配符，使搜索 "100%" 匹配字面文本，而不是匹配所有标题。
func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

// insertImage 通过连接池或事务写入一行图片记录。
func insertImage(ctx context.Context, tx pg.Tx, image *domain.Image) error {
	const query = `
		INSERT INTO product_images (id, product_id, object_key, sort_order, created_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := tx.Exec(ctx, query, image.ID, image.ProductID, image.ObjectKey, image.SortOrder, image.CreatedAt)
	switch {
	case pg.IsUniqueViolation(err, imageSlotConstraint):
		return application.ErrImageSlotTaken
	case isForeignKeyViolation(err):
		return application.ErrNotFound
	case err != nil:
		return fmt.Errorf("postgres: insert product image: %w", err)
	}
	return nil
}

func scanProduct(row pgx.Row) (*domain.Product, error) {
	var (
		product  domain.Product
		category string
		status   string
	)
	err := row.Scan(
		&product.ID, &product.SellerID, &product.Title, &product.PriceMinor,
		&category, &product.Description, &status, &product.Version,
		&product.CreatedAt, &product.UpdatedAt,
	)
	if pg.IsNoRows(err) {
		return nil, application.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	product.Category = domain.Category(category)
	product.Status = domain.Status(status)
	return &product, nil
}

func collectProducts(rows pgx.Rows) ([]*domain.Product, error) {
	defer rows.Close()

	var products []*domain.Product
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: read products: %w", err)
	}
	return products, nil
}
