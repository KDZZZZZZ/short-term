import { motion } from 'motion/react'
import { Avatar, Badge, Button, Dropdown, Separator, toast } from '@heroui/react'
import {
  MessageCircle,
  Moon,
  Plus,
  ScrollText,
  Store,
  Sun,
  User,
  LogOut,
} from 'lucide-react'
import { MarketStallIcon } from '@/components/icons/koboyo'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { logoutUser } from '@/lib/api/auth'
import { nicknameInitial } from '@/lib/format'
import { useUnreadCount } from '@/hooks/use-unread-count'
import { useAuthStore } from '@/stores/auth-store'
import { useThemeStore } from '@/stores/theme-store'

interface NavLinkProps {
  to: string
  current: string
  icon: React.ReactNode
  label: string
  badge?: number
}

function NavLink({ to, current, icon, label, badge }: NavLinkProps) {
  const active = current === to || (to !== '/' && current.startsWith(to))
  return (
    <Link
      className="group relative flex items-center rounded-lg px-3 py-2 text-sm font-medium outline-none transition-colors duration-200 hover:bg-header-hover focus-visible:ring-2 focus-visible:ring-header-active data-[active=true]:text-header-active-foreground data-[active=false]:text-header-foreground/85 hover:data-[active=false]:text-header-foreground"
      data-active={active ? 'true' : 'false'}
      to={to}
    >
      {/* 选中态：pill 跨导航项平滑滑动（layoutId 共享同一元素） */}
      {active ? (
        <motion.span
          aria-hidden
          className="absolute inset-0 rounded-lg bg-header-active"
          layoutId="nav-active-pill"
          transition={{ type: 'spring', stiffness: 500, damping: 40 }}
        />
      ) : null}
      <span className="relative z-10 flex items-center gap-1.5">
        <span className="transition-transform duration-200 group-hover:scale-110">{icon}</span>
        <span className="hidden sm:inline">{label}</span>
        {badge && badge > 0 ? (
          <Badge.Anchor>
            <span />
            <Badge color="danger" size="sm">
              {badge > 99 ? '99+' : badge}
            </Badge>
          </Badge.Anchor>
        ) : null}
      </span>
    </Link>
  )
}

export function AppLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const user = useAuthStore((state) => state.user)
  const clear = useAuthStore((state) => state.clear)
  const { mode, toggle } = useThemeStore()
  const { data: unread } = useUnreadCount()

  const handleLogout = async () => {
    try {
      await logoutUser()
    } catch {
      // 令牌已失效等情况：本地退出即可
    }
    clear()
    toast.success('已退出登录')
    navigate('/login', { replace: true })
  }

  return (
    <div className="flex min-h-full flex-col bg-background text-foreground">
      <header className="sticky top-0 z-40 border-b border-header-border bg-header backdrop-blur">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center gap-2 px-4">
          <Link className="flex items-center gap-2 text-base font-bold text-header-foreground" to="/">
            <MarketStallIcon className="h-6 w-auto text-header-active" />
            <span className="hidden sm:inline">校园二手集市</span>
          </Link>

          <Separator className="mx-2 hidden h-6 bg-header-foreground/25 sm:block" orientation="vertical" />

          <nav className="flex flex-1 items-center gap-1">
            <NavLink current={location.pathname} icon={<Store className="size-4" />} label="市场" to="/" />
            <NavLink
              current={location.pathname}
              icon={<MessageCircle className="size-4" />}
              label="消息"
              to="/chats"
              badge={unread?.unread_count ?? 0}
            />
            <NavLink
              current={location.pathname}
              icon={<ScrollText className="size-4" />}
              label="交易"
              to="/trades"
            />
            <NavLink
              current={location.pathname}
              icon={<User className="size-4" />}
              label="我的"
              to="/my/products"
            />
          </nav>

          <Button
            isIconOnly
            aria-label={mode === 'dark' ? '切换到浅色模式' : '切换到深色模式'}
            className="bg-transparent text-header-foreground hover:bg-header-hover"
            size="sm"
            variant="tertiary"
            onPress={toggle}
          >
            <motion.span
              animate={{ rotate: 0, opacity: 1 }}
              className="flex"
              initial={{ rotate: -120, opacity: 0 }}
              key={mode}
              transition={{ duration: 0.3, ease: [0.21, 0.68, 0.35, 1] }}
            >
              {mode === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
            </motion.span>
          </Button>

          <Button
            className="bg-header-active text-header-active-foreground shadow-none hover:bg-header-active-hover"
            onPress={() => navigate('/products/new')}
            size="sm"
          >
            <Plus className="size-4" />
            <span className="hidden sm:inline">发布</span>
          </Button>

          <Dropdown>
            <Button
              aria-label="账号菜单"
              className="bg-transparent text-header-foreground hover:bg-header-hover"
              isIconOnly
              size="sm"
              variant="tertiary"
            >
              <Avatar className="size-7">
                <Avatar.Fallback>{nicknameInitial(user?.nickname ?? '我')}</Avatar.Fallback>
              </Avatar>
            </Button>
            <Dropdown.Popover placement="bottom end">
              <Dropdown.Menu
                onAction={(key) => {
                  if (key === 'profile') navigate('/profile')
                  if (key === 'my-products') navigate('/my/products')
                  if (key === 'favorites') navigate('/favorites')
                  if (key === 'logout') void handleLogout()
                }}
              >
                <Dropdown.Item id="profile" textValue="个人资料">
                  <User className="size-4" />
                  个人资料
                </Dropdown.Item>
                <Dropdown.Item id="my-products" textValue="我的商品">
                  <Store className="size-4" />
                  我的商品
                </Dropdown.Item>
                <Dropdown.Item id="favorites" textValue="我的收藏">
                  <Store className="size-4" />
                  我的收藏
                </Dropdown.Item>
                <Dropdown.Item id="logout" textValue="退出登录" variant="danger">
                  <LogOut className="size-4" />
                  退出登录
                </Dropdown.Item>
              </Dropdown.Menu>
            </Dropdown.Popover>
          </Dropdown>
        </div>
      </header>

      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6">
        <motion.div
          animate={{ opacity: 1, y: 0 }}
          initial={{ opacity: 0, y: 6 }}
          key={location.pathname}
          transition={{ duration: 0.2, ease: 'easeOut' }}
        >
          <Outlet />
        </motion.div>
      </main>
    </div>
  )
}
