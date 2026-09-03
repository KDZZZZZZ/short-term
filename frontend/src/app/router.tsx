import { createBrowserRouter } from 'react-router-dom'
import { GuestOnly } from '@/components/guest-only'
import { RequireAuth } from '@/components/require-auth'
import { AppLayout } from '@/components/layout/app-layout'
import { ChatPage } from '@/pages/chat/chat-page'
import { ConversationsPage } from '@/pages/chat/conversations-page'
import { FavoritesPage } from '@/pages/favorites/favorites-page'
import { LoginPage } from '@/pages/auth/login-page'
import { MarketPage } from '@/pages/market/market-page'
import { NotFoundPage } from '@/pages/not-found-page'
import { EditProductPage } from '@/pages/product/edit-product-page'
import { ManageImagesPage } from '@/pages/product/manage-images-page'
import { MyProductsPage } from '@/pages/product/my-products-page'
import { ProductDetailPage } from '@/pages/product/product-detail-page'
import { PublishProductPage } from '@/pages/product/publish-product-page'
import { ProfilePage } from '@/pages/profile/profile-page'
import { RegisterPage } from '@/pages/auth/register-page'
import { TradesPage } from '@/pages/trades/trades-page'

export const router = createBrowserRouter([
  {
    path: '/login',
    element: (
      <GuestOnly>
        <LoginPage />
      </GuestOnly>
    ),
  },
  {
    path: '/register',
    element: (
      <GuestOnly>
        <RegisterPage />
      </GuestOnly>
    ),
  },
  {
    element: (
      <RequireAuth>
        <AppLayout />
      </RequireAuth>
    ),
    children: [
      { index: true, element: <MarketPage /> },
      { path: 'products/new', element: <PublishProductPage /> },
      { path: 'products/:productId', element: <ProductDetailPage /> },
      { path: 'products/:productId/edit', element: <EditProductPage /> },
      { path: 'products/:productId/images', element: <ManageImagesPage /> },
      { path: 'my/products', element: <MyProductsPage /> },
      { path: 'favorites', element: <FavoritesPage /> },
      { path: 'chats', element: <ConversationsPage /> },
      { path: 'chats/:conversationId', element: <ChatPage /> },
      { path: 'trades', element: <TradesPage /> },
      { path: 'profile', element: <ProfilePage /> },
      { path: '*', element: <NotFoundPage /> },
    ],
  },
])
