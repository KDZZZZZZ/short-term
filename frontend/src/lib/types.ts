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

export interface SellerContact {
  id: Identifier
  nickname: string
  wechat: string | null
  qq: string | null
}

export interface UserMe {
  id: Identifier
  student_no: StudentNumber
  nickname: string
  wechat: string | null
  qq: string | null
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
  buyer: UserPublic
  seller: UserPublic
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

export type TradeRole = 'buyer' | 'seller'

export interface TradeCreateRequest {
  conversation_id?: string | null
}

export interface ReasonRequest {
  reason: string
}

export type ProductStatusFilter = ProductStatus // /users/me/products?status=
export type CategoryFilter = ProductCategory
