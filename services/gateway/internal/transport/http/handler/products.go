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

// Products 提供 /products 端点和 /users/me/products。
type Products struct {
	marketplace marketplacev1.MarketplaceServiceClient
	accounts    accountv1.AccountServiceClient
	aggregator  *aggregation.Aggregator
	responder   Responder
}

// NewProducts 构造商品处理器。
func NewProducts(marketplace marketplacev1.MarketplaceServiceClient, accounts accountv1.AccountServiceClient, aggregator *aggregation.Aggregator, responder Responder) *Products {
	return &Products{marketplace: marketplace, accounts: accounts, aggregator: aggregator, responder: responder}
}

// List 处理 GET /products。
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

// ListMine 处理 GET /users/me/products。
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

	h.respondWithMyPage(w, r, resp.GetPage())
}

// ListByUser 处理 GET /users/{userId}/products。
//
// 公开列表只返回 ON_SALE 与 SOLD 商品：在售商品用于浏览，已售出商品用于在
// 详情中展示买家评价；RESERVED 和 OFF_SHELF 只有卖家自己可见。
func (h *Products) ListByUser(w http.ResponseWriter, r *http.Request) {
	if _, err := h.accounts.GetUser(h.downstream(r), &accountv1.GetUserRequest{
		UserId: r.PathValue("userId"),
	}); err != nil {
		// 对不存在的用户不区分“无商品”和“用户不存在”，契约规定后者返回 404。
		h.responder.Error(w, r, err)
		return
	}

	page, size, ok := h.pagination(w, r)
	if !ok {
		return
	}

	resp, err := h.marketplace.ListUserProducts(h.downstream(r), &marketplacev1.ListUserProductsRequest{
		SellerId: r.PathValue("userId"),
		Statuses: []marketplacev1.ProductStatus{
			marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE,
			marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD,
		},
		Page:     page,
		PageSize: size,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithPage(w, r, resp.GetPage())
}

// Create 处理 POST /products。
func (h *Products) Create(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.ActorID(r.Context())

	// 契约要求发布者至少有一种联系方式，因为买家只能通过微信或 QQ 联系卖家。
	// 该事实由 Account Service 负责，因此在保存任何内容前检查。
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

// Get 处理 GET /products/{productId}。
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

// Update 处理 PATCH /products/{productId}。
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

// AddImages 处理 POST /products/{productId}/images。
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

// DeleteImage 处理 DELETE /products/{productId}/images/{imageId}。
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

// OffShelf 处理 POST /products/{productId}/off-shelf。
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

// Relist 处理 POST /products/{productId}/relist。
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

// --- 共享步骤 ---------------------------------------------------------------

// respondWithPage 通过一次批量调用补全卖家身份并写入页面。
// 每页调用一次，绝不按行调用。
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

// respondWithMyPage 写入我的商品页：补全卖家身份，并按页批量读取每件商品
// 收到的买家评价、补全买家昵称。评价最多每商品一条，因此按商品标识索引。
func (h *Products) respondWithMyPage(w http.ResponseWriter, r *http.Request, page *marketplacev1.ProductPage) {
	ctx := h.downstream(r)

	productIDs := make([]string, 0, len(page.GetItems()))
	sellerIDs := make([]string, 0, len(page.GetItems()))
	for _, item := range page.GetItems() {
		productIDs = append(productIDs, item.GetId())
		sellerIDs = append(sellerIDs, item.GetSellerId())
	}

	sellers, err := h.aggregator.Users(ctx, sellerIDs)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	batch, err := h.marketplace.BatchGetProductTradeReviews(ctx, &marketplacev1.BatchGetProductTradeReviewsRequest{
		ProductIds: productIDs,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	reviews := batch.GetReviews()

	buyers, err := h.aggregator.Users(ctx, mapper.TradeReviewBuyerIDs(reviews))
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.OK(w, r, mapper.MyProductPage(page, sellers, reviews, buyers))
}

// respondWithDetail 补全卖家联系方式、收藏标记和买家评价。
//
// 前两者都是 ProductDetail 的必填字段，因此获取任一字段失败都会使请求失败，
// 不会返回填充虚构值的响应（docs/software-design.md 第 8.3 节）。买家评价
// 只存在于已完成交易的商品上，因此仅对 SOLD 商品批量读取一次。
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

	var review *marketplacev1.TradeReview
	var reviewers map[string]*accountv1.UserPublic
	if product.GetStatus() == marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD {
		batch, err := h.marketplace.BatchGetProductTradeReviews(ctx, &marketplacev1.BatchGetProductTradeReviewsRequest{
			ProductIds: []string{product.GetId()},
		})
		if err != nil {
			h.responder.Error(w, r, err)
			return
		}
		review = batch.GetReviews()[product.GetId()]
		if review != nil {
			reviewers, err = h.aggregator.Users(ctx, []string{review.GetBuyerId()})
			if err != nil {
				h.responder.Error(w, r, err)
				return
			}
		}
	}

	h.responder.Success(w, r, status, mapper.ProductDetail(product, mapper.SellerContact(contact), favorited, review, reviewers))
}

// hasContact 判断当前用户是否填写了微信或 QQ 联系方式。
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

// pagination 读取 page 和 page_size 查询参数。
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

// intQuery 读取一个有界整数查询参数。max 为零表示没有上界。
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

// parseMultipart 在文档规定的大小限制内读取 multipart 正文。
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

// formImages 读取上传文件，在任何字节到达 Marketplace Service 前检查文件数量和
// 单文件大小限制。
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
			// 声明的类型只作为提示转发；Marketplace Service 根据字节内容判断真实类型。
			ContentType: header.Header.Get("Content-Type"),
		})
	}
	return uploads, true
}

// formPrice 读取并转换必填的价格字段。
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

// formCategory 读取并转换分类字段。
func (h *Products) formCategory(w http.ResponseWriter, r *http.Request, form *multipart.Form) (marketplacev1.ProductCategory, bool) {
	category, valid := mapper.ParseProductCategory(formValue(form, "category"))
	if !valid {
		h.responder.Fail(w, r, errs.CodeValidation, "商品分类不合法")
		return category, false
	}
	return category, true
}

// downstream 构造内部调用的上下文，携带当前用户和公开请求标识。
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
