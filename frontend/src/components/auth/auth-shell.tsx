import type { ReactNode } from 'react'
import { Store } from 'lucide-react'
import { Link } from 'react-router-dom'

/**
 * 登录/注册共用分栏布局（参考 Human Design 设计稿）：
 * 左侧白色表单面板 + 右侧品牌色插画面板，移动端单栏。
 */
export function AuthShell({
  title,
  subtitle,
  children,
  footer,
}: {
  title: string
  subtitle: string
  children: ReactNode
  footer: ReactNode
}) {
  return (
    <div className="flex min-h-full items-center justify-center bg-background px-4 py-8 sm:px-6">
      <div className="grid w-full max-w-4xl overflow-hidden rounded-2xl border border-border-secondary bg-surface shadow-sm lg:grid-cols-[minmax(0,460px)_minmax(0,1fr)]">
        {/* 左：表单面板 */}
        <div className="flex flex-col p-8 sm:p-10">
          <Link className="flex items-center gap-2 self-start" to="/">
            <Store className="size-5 text-accent" />
            <span className="text-lg font-bold text-foreground">校园二手集市</span>
          </Link>

          <div className="flex flex-1 flex-col justify-center py-10">
            <h1 className="text-center text-3xl font-bold text-foreground">{title}</h1>
            <p className="mb-8 mt-2 text-center text-sm text-muted">{subtitle}</p>
            {children}
          </div>

          <div className="text-center text-sm text-muted">{footer}</div>
        </div>

        {/* 右：品牌插画面板 */}
        <div className="relative hidden items-center justify-center overflow-hidden bg-accent-soft lg:flex">
          <div className="absolute -left-16 -top-16 size-64 rounded-full bg-accent/15" />
          <div className="absolute -bottom-20 -right-16 size-80 rounded-full bg-accent/10" />
          <div className="absolute bottom-24 left-16 size-16 rounded-full bg-accent/20" />
          <div className="relative z-10 flex flex-col items-center gap-6">
            <div className="flex size-40 items-center justify-center rounded-3xl bg-accent shadow-lg">
              <Store className="size-20 text-accent-foreground" />
            </div>
            <div className="text-center">
              <p className="text-xl font-bold text-accent-soft-foreground">让闲置好物，遇见新主人</p>
              <p className="mt-1 text-sm text-accent-soft-foreground/70">校园二手集市 · 学号即可注册</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
