// Package postgres implements the Marketplace repository ports against the
// service's own database.
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

// imageSlotConstraint is the unique index that serialises two writers racing
// for the same image slot.
const imageSlotConstraint = "product_images_slot_unique"

// productColumns is the projection every product read shares.
const productColumns = `id, seller_id, title, price_minor, category, description, status, version, created_at, updated_at`

// ProductRepository stores products and their images in PostgreSQL.
type ProductRepository struct {
	pool *pgxpool.Pool
}

// NewProductRepository builds a repository over an open pool.
func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

var _ application.ProductRepository = (*ProductRepository)(nil)

// Create inserts a product and its initial images in one transaction, so a
// listing is never visible without the images it was published with.
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

// ByID loads one product with its images.
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

// ByIDs loads several products with their images in two round trips.
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

// List returns one page of products in a deterministic order.
func (r *ProductRepository) List(ctx context.Context, filter application.ProductFilter, page application.Page) (application.ProductPage, error) {
	where, args := buildFilter(filter)

	countQuery := `SELECT count(*) FROM products` + where
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return application.ProductPage{}, fmt.Errorf("postgres: count products: %w", err)
	}

	// created_at alone is not unique, so id is the tie-breaker. Without it a
	// page boundary could repeat or skip products published in the same
	// instant (docs/software-design.md section 8.2).
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

// Update writes the product back only if its stored version still matches the
// one that was read, which is how a lost update is detected.
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

// AddImage inserts one image.
func (r *ProductRepository) AddImage(ctx context.Context, image *domain.Image) error {
	return insertImage(ctx, r.pool, image)
}

// DeleteImage removes one image and returns its object key.
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

// explainMissingUpdate distinguishes a product that no longer exists from one
// that another writer changed first.
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

// attachImages loads the images of every product in one query.
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

// imagesFor loads images for several products at once, avoiding one query per
// row in list responses.
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

// buildFilter turns a filter into a WHERE clause and its arguments.
func buildFilter(filter application.ProductFilter) (string, []any) {
	var conditions []string
	var args []any

	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
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
		// The keyword is a literal substring, so LIKE metacharacters in user
		// input are escaped rather than interpreted as a pattern.
		args = append(args, "%"+escapeLike(*filter.Keyword)+"%")
		conditions = append(conditions, fmt.Sprintf(`title ILIKE $%d ESCAPE '\'`, len(args)))
	}

	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

// escapeLike neutralises the LIKE wildcards so a search for "100%" matches the
// literal text rather than every title.
func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

// insertImage writes one image row through either the pool or a transaction.
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
