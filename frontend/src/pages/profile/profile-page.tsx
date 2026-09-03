import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import {
  Avatar,
  Button,
  Card,
  Description,
  Input,
  Label,
  Separator,
  TextField,
  toast,
} from '@heroui/react'
import { LogOut } from 'lucide-react'
import {
  changeCurrentUserPassword,
  getCurrentUser,
  logoutUser,
  updateCurrentUser,
} from '@/lib/api/auth'
import { isApiError } from '@/lib/http'
import { QQ_PATTERN, WECHAT_PATTERN, formatDateTime, nicknameInitial } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

export function ProfilePage() {
  const navigate = useNavigate()
  const setUser = useAuthStore((state) => state.setUser)
  const clear = useAuthStore((state) => state.clear)
  const cachedUser = useAuthStore((state) => state.user)

  const { data: user } = useQuery({
    queryKey: ['me'],
    queryFn: getCurrentUser,
    initialData: cachedUser ?? undefined,
  })

  // 资料表单：空字符串表示“保持原值”，对应契约中省略字段
  const [nickname, setNickname] = useState(user?.nickname ?? '')
  const [wechat, setWechat] = useState('')
  const [qq, setQq] = useState('')
  const [profileError, setProfileError] = useState<string | null>(null)
  const [profileFormKey, setProfileFormKey] = useState(0)

  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [passwordError, setPasswordError] = useState<string | null>(null)

  const profileMutation = useMutation({
    mutationFn: () => {
      const body: { nickname?: string; wechat?: string; qq?: string } = {}
      const trimmedNickname = nickname.trim()
      const trimmedWechat = wechat.trim()
      const trimmedQq = qq.trim()
      if (trimmedNickname && trimmedNickname !== user?.nickname) body.nickname = trimmedNickname
      if (trimmedWechat) body.wechat = trimmedWechat
      if (trimmedQq) body.qq = trimmedQq
      if (Object.keys(body).length === 0) throw new Error('没有需要保存的修改')
      return updateCurrentUser(body)
    },
    onSuccess: (updated) => {
      setUser(updated)
      setWechat('')
      setQq('')
      setProfileError(null)
      setProfileFormKey((key) => key + 1)
      toast.success('资料已更新')
    },
    onError: (err) => {
      setProfileError(err instanceof Error && !(err instanceof TypeError) ? err.message : isApiError(err) ? err.message : '保存失败，请稍后重试')
    },
  })

  const passwordMutation = useMutation({
    mutationFn: () => changeCurrentUserPassword({ old_password: oldPassword, new_password: newPassword }),
    onSuccess: () => {
      setOldPassword('')
      setNewPassword('')
      setConfirmPassword('')
      setPasswordError(null)
      toast.success('密码已修改')
    },
    onError: (err) => {
      setPasswordError(isApiError(err) ? err.message : '修改失败，请稍后重试')
    },
  })

  const submitProfile = (event: React.FormEvent) => {
    event.preventDefault()
    const trimmedNickname = nickname.trim()
    if (trimmedNickname && (trimmedNickname.length < 1 || trimmedNickname.length > 50)) {
      setProfileError('昵称长度需在 1-50 字之间')
      return
    }
    const trimmedWechat = wechat.trim()
    const trimmedQq = qq.trim()
    if (trimmedWechat && !WECHAT_PATTERN.test(trimmedWechat)) {
      setProfileError('微信号不能包含空白字符，长度 1-64')
      return
    }
    if (trimmedQq && !QQ_PATTERN.test(trimmedQq)) {
      setProfileError('QQ 号需为 5-20 位数字')
      return
    }
    setProfileError(null)
    profileMutation.mutate()
  }

  const submitPassword = (event: React.FormEvent) => {
    event.preventDefault()
    if (newPassword.length < 8 || newPassword.length > 64) {
      setPasswordError('新密码长度需在 8-64 位之间')
      return
    }
    if (newPassword !== confirmPassword) {
      setPasswordError('两次输入的新密码不一致')
      return
    }
    setPasswordError(null)
    passwordMutation.mutate()
  }

  const handleLogout = async () => {
    try {
      await logoutUser()
    } catch {
      // 本地退出即可
    }
    clear()
    toast.success('已退出登录')
    navigate('/login', { replace: true })
  }

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <h1 className="text-lg font-bold">个人资料</h1>

      <Card className="flex flex-col gap-4 p-4 sm:p-6">
        <div className="flex items-center gap-4">
          <Avatar className="size-14">
            <Avatar.Fallback>{nicknameInitial(user?.nickname ?? '我')}</Avatar.Fallback>
          </Avatar>
          <div className="flex flex-col gap-0.5">
            <span className="text-base font-semibold">{user?.nickname}</span>
            <span className="text-sm text-muted">学号 {user?.student_no}</span>
            <span className="text-xs text-muted">
              注册于 {user ? formatDateTime(user.created_at) : ''}
            </span>
          </div>
        </div>

        <Separator />

        <form className="flex flex-col gap-4" key={profileFormKey} onSubmit={submitProfile}>
          <TextField className="w-full" name="nickname" value={nickname} onChange={setNickname}>
            <Label>昵称</Label>
            <Input maxLength={50} placeholder={user?.nickname ?? '校园用户'} />
            <Description>留空表示保持原值</Description>
          </TextField>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <TextField className="w-full" name="wechat" value={wechat} onChange={setWechat}>
              <Label>微信</Label>
              <Input placeholder={user?.wechat ? `当前：${user.wechat}` : '尚未填写'} />
              <Description>填写后不能清空，只能改为其他非空值</Description>
            </TextField>
            <TextField className="w-full" name="qq" value={qq} onChange={setQq}>
              <Label>QQ</Label>
              <Input inputMode="numeric" placeholder={user?.qq ? `当前：${user.qq}` : '尚未填写'} />
              <Description>发布商品需至少填写微信或 QQ 之一</Description>
            </TextField>
          </div>

          {profileError ? <p className="text-sm text-danger">{profileError}</p> : null}

          <div className="flex justify-end">
            <Button isDisabled={profileMutation.isPending} isPending={profileMutation.isPending} type="submit">
              保存资料
            </Button>
          </div>
        </form>
      </Card>

      <Card className="flex flex-col gap-4 p-4 sm:p-6">
        <h2 className="text-sm font-semibold">修改密码</h2>
        <form className="flex flex-col gap-4" onSubmit={submitPassword}>
          <TextField className="w-full" name="old_password" value={oldPassword} onChange={setOldPassword}>
            <Label>当前密码</Label>
            <Input autoComplete="current-password" type="password" />
          </TextField>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <TextField className="w-full" name="new_password" value={newPassword} onChange={setNewPassword}>
              <Label>新密码</Label>
              <Input autoComplete="new-password" placeholder="8-64 位" type="password" />
            </TextField>
            <TextField className="w-full" name="confirm_password" value={confirmPassword} onChange={setConfirmPassword}>
              <Label>确认新密码</Label>
              <Input autoComplete="new-password" type="password" />
            </TextField>
          </div>

          {passwordError ? <p className="text-sm text-danger">{passwordError}</p> : null}

          <div className="flex justify-end">
            <Button isDisabled={passwordMutation.isPending} isPending={passwordMutation.isPending} type="submit">
              修改密码
            </Button>
          </div>
        </form>

        <Separator />

        <Button className="self-start" variant="danger-soft" onPress={() => void handleLogout()}>
          <LogOut className="size-4" />
          退出登录
        </Button>
      </Card>
    </div>
  )
}
