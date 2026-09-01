package handler

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/mapper"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

// Products serves the /products endpoints and /users/me/products.
type Products struct {
	marketplace marketplacev1.MarketplaceServiceClient
	accounts    accountv1.AccountServiceClient
	aggregator  *aggregation.Aggregator
	responder   Responder
}

// NewProducts builds the product handler.
func NewProducts(marketplace marketplacev1.MarketplaceServiceClient, accounts accountv1.AccountServiceClient, aggregator *aggregation.Aggregator, responder Responder) *Products {
	return &Products{marketplace: marketplace, accounts: accounts, aggregator: aggregator, responder: responder}
}

// List handles GET /products.
func (h *Products) List(w http.ResponseWriter, r *http.Request) {
	page, size, ok := h.pagination(w, r)
	if !ok {
		return
	}

	req := &marketplacev1.ListProductsRequest{Page: page, PageSize: size}

	if raw := r.URL.Query().Get("keyword"); raw != "" {
		keyword := strings.TrimSpace(raw)
		if keyword == "" || len([]rune(keyword)) > 100 {
			h.responder.Fail(w, r, errs.CodeValidation, "关键词长度必须为 1 至 100 个字符")
			return
		}
		req.Keyword = &keyword
	}
	if raw := r.URL.Query().Get("category"); raw != "" {
		category, valid := mapper.ParseProductCategory(raw)
		if !valid {
			h.responder.Fail(w, r, errs.CodeValidation, "商品分类不合法")
			return
		}
		req.Category = &category
	}

	resp, err := h.marketplace.ListProducts(h.downstream(r), req)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithPage(w, r, resp.GetPage())
}

// ListMine handles GET /users/me/products.
func (h *Products) ListMine(w http.ResponseWriter, r *http.Request) {
	page, size, ok := h.pagination(w, r)
	if !ok {
		return
	}

	req := &marketplacev1.ListUserProductsRequest{
		SellerId: middleware.ActorID(r.Context()),
		Page:     page,
		PageSize: size,
	}
	if raw := r.URL.Query().Get("status"); raw != "" {
		status, valid := mapper.ParseProductStatus(raw)
		if !valid {
			h.responder.Fail(w, r, errs.CodeValidation, "商品状态不合法")
			return
		}
		req.Status = &status
	}

	resp, err := h.marketplace.ListUserProducts(h.downstream(r), req)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithPage(w, r, resp.GetPage())
}

// Create handles POST /products.
func (h *Products) Create(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.ActorID(r.Context())

	// The contract requires a publisher to have a contact method, because a
	// buyer's only way to reach them is WeChat or QQ. The Account Service owns
	// that fact, so it is checked before anything is stored.
	if !h.hasContact(w, r, actorID) {
		return
	}

	form, ok := h.parseMultipart(w, r)
	if !ok {
		return
	}
	defer func() { _ = form.RemoveAll() }()

	priceMinor, ok := h.formPrice(w, r, form)
	if !ok {
		return
	}
	category, ok := h.formCategory(w, r, form)
	if !ok {
		return
	}
	images, ok := h.formImages(w, r, form, false)
	if !ok {
		return
	}

	resp, err := h.marketplace.CreateProduct(h.downstream(r), &marketplacev1.CreateProductRequest{
		ActorId:     actorID,
		Title:       formValue(form, "title"),
		PriceMinor:  priceMinor,
		Category:    category,
		Description: formValue(form, "description"),
		Images:      images,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithDetail(w, r, resp.GetProduct(), http.StatusCreated)
}

// Get handles GET /products/{productId}.
func (h *Products) Get(w http.ResponseWriter, r *http.Request) {
	resp, err := h.marketplace.GetProduct(h.downstream(r), &marketplacev1.GetProductRequest{
		ProductId: r.PathValue("productId"),
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithDetail(w, r, resp.GetProduct(), http.StatusOK)
}

// Update handles PATCH /products/{productId}.
func (h *Products) Update(w http.ResponseWriter, r *http.Request) {
	var body dto.ProductUpdateRequest
	if !decodeJSON(w, r, h.responder, &body) {
		return
	}
	if body.Title == nil && body.Price == nil && body.Category == nil && body.Description == nil {
		h.responder.Fail(w, r, errs.CodeValidation, "请至少修改一项商品信息")
		return
	}

	req := &marketplacev1.UpdateProductRequest{
		ActorId:     middleware.ActorID(r.Context()),
		ProductId:   r.PathValue("productId"),
		Title:       body.Title,
		Description: body.Description,
	}
	if body.Price != nil {
		priceMinor, err := mapper.ParsePrice(*body.Price)
		if err != nil {
			h.responder.Fail(w, r, errs.CodeValidation, "价格格式不合法")
			return
		}
		req.PriceMinor = &priceMinor
	}
	if body.Category != nil {
		category, valid := mapper.ParseProductCategory(*body.Category)
		if !valid {
			h.responder.Fail(w, r, errs.CodeValidation, "商品分类不合法")
			return
		}
		req.Category = &category
	}

	resp, err := h.marketplace.UpdateProduct(h.downstream(r), req)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithDetail(w, r, resp.GetProduct(), http.StatusOK)
}

// AddImages handles POST /products/{productId}/images.
func (h *Products) AddImages(w http.ResponseWriter, r *http.Request) {
	form, ok := h.parseMultipart(w, r)
	if !ok {
		return
	}
	defer func() { _ = form.RemoveAll() }()

	images, ok := h.formImages(w, r, form, true)
	if !ok {
		return
	}

	resp, err := h.marketplace.AddProductImages(h.downstream(r), &marketplacev1.AddProductImagesRequest{
		ActorId:   middleware.ActorID(r.Context()),
		ProductId: r.PathValue("productId"),
		Images:    images,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.Created(w, r, dto.ProductImageList{Images: mapper.ProductImages(resp.GetImages())})
}

// DeleteImage handles DELETE /products/{productId}/images/{imageId}.
func (h *Products) DeleteImage(w http.ResponseWriter, r *http.Request) {
	_, err := h.marketplace.DeleteProductImage(h.downstream(r), &marketplacev1.DeleteProductImageRequest{
		ActorId:   middleware.ActorID(r.Context()),
		ProductId: r.PathValue("productId"),
		ImageId:   r.PathValue("imageId"),
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.Empty(w, r)
}

// OffShelf handles POST /products/{productId}/off-shelf.
func (h *Products) OffShelf(w http.ResponseWriter, r *http.Request) {
	resp, err := h.marketplace.OffShelfProduct(h.downstream(r), &marketplacev1.OffShelfProductRequest{
		ActorId:   middleware.ActorID(r.Context()),
		ProductId: r.PathValue("productId"),
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithDetail(w, r, resp.GetProduct(), http.StatusOK)
}

// Relist handles POST /products/{productId}/relist.
func (h *Products) Relist(w http.ResponseWriter, r *http.Request) {
	resp, err := h.marketplace.RelistProduct(h.downstream(r), &marketplacev1.RelistProductRequest{
		ActorId:   middleware.ActorID(r.Context()),
		ProductId: r.PathValue("productId"),
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithDetail(w, r, resp.GetProduct(), http.StatusOK)
}

// --- shared steps -----------------------------------------------------------

// respondWithPage completes seller identities in one batch call and writes the
// page. One call per page, never one per row.
func (h *Products) respondWithPage(w http.ResponseWriter, r *http.Request, page *marketplacev1.ProductPage) {
	sellerIDs := make([]string, 0, len(page.GetItems()))
	for _, item := range page.GetItems() {
		sellerIDs = append(sellerIDs, item.GetSellerId())
	}

	sellers, err := h.aggregator.Users(h.downstream(r), sellerIDs)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.OK(w, r, mapper.ProductPage(page, sellers))
}

// respondWithDetail completes the seller contact and the favorite flag.
//
// Both are required fields of ProductDetail, so a failure to fetch either
// fails the request rather than returning a response with invented values
// (docs/software-design.md section 8.3).
func (h *Products) respondWithDetail(w http.ResponseWriter, r *http.Request, product *marketplacev1.ProductDetail, status int) {
	ctx := h.downstream(r)

	contact, err := h.aggregator.SellerContact(ctx, product.GetSellerId())
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	favorited, err := h.aggregator.IsFavorited(ctx, middleware.ActorID(r.Context()), product.GetId())
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.Success(w, r, status, mapper.ProductDetail(product, mapper.SellerContact(contact), favorited))
}

// hasContact reports whether the acting user published a WeChat or QQ contact.
func (h *Products) hasContact(w http.ResponseWriter, r *http.Request, actorID string) bool {
	profile, err := h.accounts.GetUser(h.downstream(r), &accountv1.GetUserRequest{UserId: actorID})
	if err != nil {
		h.responder.Error(w, r, err)
		return false
	}
	if profile.GetUser().GetWechat() == "" && profile.GetUser().GetQq() == "" {
		h.responder.Fail(w, r, errs.CodeContactRequired, "请先在个人资料中填写微信或 QQ")
		return false
	}
	return true
}

// pagination reads the page and page_size query parameters.
func (h *Products) pagination(w http.ResponseWriter, r *http.Request) (page, size int32, ok bool) {
	page, ok = h.intQuery(w, r, "page", 1, 1, 0)
	if !ok {
		return 0, 0, false
	}
	size, ok = h.intQuery(w, r, "page_size", 20, 1, 100)
	if !ok {
		return 0, 0, false
	}
	return page, size, true
}

// intQuery reads one bounded integer query parameter. A max of zero means no
// upper bound.
func (h *Products) intQuery(w http.ResponseWriter, r *http.Request, name string, fallback, minimum, maximum int32) (int32, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, true
	}

	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || int32(value) < minimum || (maximum > 0 && int32(value) > maximum) {
		h.responder.Fail(w, r, errs.CodeValidation, name+" 参数不合法")
		return 0, false
	}
	return int32(value), true
}

// parseMultipart reads a multipart body within the documented size limit.
func (h *Products) parseMultipart(w http.ResponseWriter, r *http.Request) (*multipart.Form, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)

	if err := r.ParseMultipartForm(multipartMemory); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.responder.Fail(w, r, errs.CodePayloadTooLarge, "上传内容超过限制")
			return nil, false
		}
		h.responder.Fail(w, r, errs.CodeValidation, "请求必须是 multipart/form-data")
		return nil, false
	}
	return r.MultipartForm, true
}

// formImages reads the uploaded files, enforcing the count and per-file size
// limits before any byte reaches the Marketplace Service.
func (h *Products) formImages(w http.ResponseWriter, r *http.Request, form *multipart.Form, required bool) ([]*marketplacev1.ImageUpload, bool) {
	files := form.File["images"]
	if len(files) == 0 {
		if required {
			h.responder.Fail(w, r, errs.CodeValidation, "请至少上传一张图片")
			return nil, false
		}
		return nil, true
	}
	if len(files) > maxImages {
		h.responder.Fail(w, r, errs.CodeImageLimitExceeded, "最多上传三张图片")
		return nil, false
	}

	uploads := make([]*marketplacev1.ImageUpload, 0, len(files))
	for _, header := range files {
		data, err := readUpload(header)
		switch {
		case errors.Is(err, errUploadTooLarge):
			h.responder.Fail(w, r, errs.CodePayloadTooLarge, "单张图片不能超过 5 MiB")
			return nil, false
		case err != nil:
			h.responder.Fail(w, r, errs.CodeValidation, "图片内容读取失败")
			return nil, false
		}

		uploads = append(uploads, &marketplacev1.ImageUpload{
			Data: data,
			// The declared type is forwarded as a hint only; the Marketplace
			// Service decides the real type from the bytes.
			ContentType: header.Header.Get("Content-Type"),
		})
	}
	return uploads, true
}

// formPrice reads and converts the required price field.
func (h *Products) formPrice(w http.ResponseWriter, r *http.Request, form *multipart.Form) (int64, bool) {
	raw := formValue(form, "price")
	if raw == "" {
		h.responder.Fail(w, r, errs.CodeValidation, "价格不能为空")
		return 0, false
	}

	priceMinor, err := mapper.ParsePrice(raw)
	if err != nil {
		h.responder.Fail(w, r, errs.CodeValidation, "价格格式不合法")
		return 0, false
	}
	return priceMinor, true
}

// formCategory reads and converts the category field.
func (h *Products) formCategory(w http.ResponseWriter, r *http.Request, form *multipart.Form) (marketplacev1.ProductCategory, bool) {
	category, valid := mapper.ParseProductCategory(formValue(form, "category"))
	if !valid {
		h.responder.Fail(w, r, errs.CodeValidation, "商品分类不合法")
		return category, false
	}
	return category, true
}

// downstream builds the context for an internal call, carrying the acting user
// and the public request identifier.
func (h *Products) downstream(r *http.Request) context.Context {
	ctx := grpcx.WithActor(r.Context(), middleware.ActorID(r.Context()))
	return grpcx.WithRequestID(ctx, middleware.RequestID(r.Context()))
}

func formValue(form *multipart.Form, name string) string {
	values := form.Value[name]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
