/**
 * 与 openapi/openapi.yaml 及其引用文件一一对应的类型定义。
 * 契约是唯一真源：字段或枚举变化时先改契约，再同步本文件。
 */

/** 服务端生成的不透明标识，客户端不得解析其结构。 */
export type Identifier = string

export type StudentNumber = string
export type Price = string // 十进制金额字符串，如 "20.00"

export type ProductCategory = 'TEXTBOOK' | 'DIGITAL' | 'LIFE' | 'OTHER'

export type ProductStatus = 'ON_SALE' | 'RESERVED' | 'SOLD' | 'OFF_SHELF'

export type TradeStatus = 'PENDING' | 'ACCEPTED' | 'COMPLETED' | 'CANCELLED'

export type ErrorCode =
  | 'VALIDATION_ERROR'
  | 'CONTACT_REQUIRED'
  | 'IMAGE_LIMIT_EXCEEDED'
  | 'PAYLOAD_TOO_LARGE'
  | 'UNAUTHORIZED'
  | 'FORBIDDEN'
  | 'RESOURCE_NOT_FOUND'
  | 'STUDENT_NO_EXISTS'
  | 'PRODUCT_NOT_AVAILABLE'
  | 'TRADE_STATE_CONFLICT'
  | 'TRADE_REVIEW_ALREADY_EXISTS'
  | 'PRODUCT_STATE_CONFLICT'
  | 'CONVERSATION_MISMATCH'
  | 'SELF_ACTION_NOT_ALLOWED'
  | 'RATE_LIMITED'
  | 'INTERNAL_ERROR'

export interface ErrorResponse {
  code: ErrorCode
  message: string
  details?: Record<string, unknown> | null
}

interface SuccessBase<T> {
  code: 'OK'
  message: 'success'
  data: T
}

export interface UserPublic {
  id: Identifier
  nickname: string
}

/** 交易一方的身份与联系方式，仅交易双方可见；未填写为 null。 */
export interface TradeParty {
  id: Identifier
  nickname: string
  wechat: string | null
  qq: string | null
}

/** 公开用户资料：只有昵称与卖家平均分，不含学号和联系方式。 */
export interface UserProfile {
  id: Identifier
  nickname: string
  average_score: string | null
}

export interface SellerContact {
  id: Identifier
  nickname: string
  wechat: string | null
  qq: string | null
  /** 卖家收到的买家评分平均值，固定两位小数；尚无评分为 null。 */
  average_score: string | null
}

export interface UserMe {
  id: Identifier
  student_no: StudentNumber
  nickname: string
  wechat: string | null
  qq: string | null
  /** 自己作为卖家收到的买家评分平均值；尚无评分为 null。 */
  average_score: string | null
  created_at: string
  updated_at: string
}

export interface AuthData {
  access_token: string
  token_type: 'Bearer'
  expires_in: number
  user: UserMe
}

export interface RegisterRequest {
  student_no: string
  password: string
  nickname?: string
  wechat?: string
  qq?: string
}

export interface LoginRequest {
  student_no: string
  password: string
}

export interface UpdateProfileRequest {
  nickname?: string
  wechat?: string
  qq?: string
}

export interface ChangePasswordRequest {
  old_password: string
  new_password: string
}

export interface ProductImage {
  id: Identifier
  url: string
  sort_order: number
  created_at: string
}

export interface ProductSummary {
  id: Identifier
  title: string
  price: Price
  category: ProductCategory
  cover_url: string | null
  status: ProductStatus
  seller: UserPublic
  created_at: string
}

export interface ProductDetail {
  id: Identifier
  title: string
  price: Price
  category: ProductCategory
  description: string
  status: ProductStatus
  images: ProductImage[]
  seller: SellerContact
  is_favorited: boolean
  buyer_review: TradeReview | null
  created_at: string
  updated_at: string
}

export interface PageData<T> {
  items: T[]
  page: number
  page_size: number
  total: number
}

export type ProductPage = PageData<ProductSummary>
export type ProductSuccess = SuccessBase<ProductDetail>
export type ProductPageSuccess = SuccessBase<ProductPage>

/** 已售出商品收到的买家评价；尚无已完成交易的买家发布时为 null。 */
export interface MyProductSummary extends ProductSummary {
  buyer_review: TradeReview | null
}
export type MyProductPage = PageData<MyProductSummary>

export interface FavoriteItem {
  product: ProductSummary
  favorited_at: string
}
export type FavoritePage = PageData<FavoriteItem>

export interface ConversationProduct {
  id: Identifier
  title: string
  cover_url: string | null
  status: ProductStatus
}

export interface LastMessage {
  id: Identifier
  sender_id: Identifier
  content: string
  created_at: string
}

export interface Conversation {
  id: Identifier
  product: ConversationProduct
  buyer: UserPublic
  seller: UserPublic
  other_user: UserPublic
  last_message: LastMessage | null
  unread_count: number
  created_at: string
  last_message_at: string | null
}

export type ConversationPage = PageData<Conversation>

export interface Message {
  id: Identifier
  conversation_id: Identifier
  sender: UserPublic
  content: string
  read_at: string | null
  created_at: string
}

export interface MessagePage {
  items: Message[] // 按创建时间倒序返回
  next_before: string | null
}

export interface SendMessageRequest {
  content: string
}

export interface MarkReadRequest {
  last_message_id: Identifier
}

export interface UnreadCountData {
  unread_count: number
}

export interface TradeProduct {
  id: Identifier
  title: string
  cover_url: string | null
  status: ProductStatus
}

export interface Trade {
  id: Identifier
  product: TradeProduct
  buyer: TradeParty
  seller: TradeParty
  conversation_id: Identifier | null
  price_snapshot: Price
  status: TradeStatus
  buyer_confirmed: boolean
  seller_confirmed: boolean
  cancel_reason: string | null
  created_at: string
  accepted_at: string | null
  completed_at: string | null
  cancelled_at: string | null
  updated_at: string
}

export type TradePage = PageData<Trade>

export interface TradeReview {
  id: Identifier
  trade_id: Identifier
  product_id: Identifier
  buyer: UserPublic
  /** 买家评分，1 至 5 的整数。 */
  score: number
  /** 买家评价文字，可选；null 表示买家未填写文字。 */
  content: string | null
  created_at: string
}

export interface TradeReviewCreateRequest {
  score: number
  content?: string | null
}

/** 公开用户评论：任何已认证用户可对任何商品发布，发布后不可改删。 */
export interface Comment {
  id: Identifier
  product_id: Identifier
  user: UserPublic
  /** 评论正文，1 至 500 个字符。 */
  content: string
  created_at: string
}

export interface CommentCreateRequest {
  content: string
}

export type CommentPage = PageData<Comment>

export type TradeRole = 'buyer' | 'seller'

export interface TradeCreateRequest {
  conversation_id?: string | null
}

export interface ReasonRequest {
  reason: string
}

export type ProductStatusFilter = ProductStatus // /users/me/products?status=
export type CategoryFilter = ProductCategory
