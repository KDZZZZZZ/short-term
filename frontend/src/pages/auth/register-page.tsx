import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, TextField, Input, Label, toast } from '@heroui/react'
import { registerUser } from '@/lib/api/auth'
import { isApiError } from '@/lib/http'
import { QQ_PATTERN, WECHAT_PATTERN } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'
import { AuthShell } from '@/components/auth/auth-shell'

export function RegisterPage() {
  const navigate = useNavigate()
  const setAuth = useAuthStore((state) => state.setAuth)

  const [form, setForm] = useState({
    student_no: '',
    password: '',
    confirm: '',
    nickname: '',
    wechat: '',
    qq: '',
  })
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const update = (key: keyof typeof form) => (value: string) =>
    setForm((prev) => ({ ...prev, [key]: value }))

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    const studentNo = form.student_no.trim()
    if (!/^[A-Za-z0-9_-]{4,32}$/.test(studentNo)) {
      setError('学号需为 4-32 位字母、数字、下划线或连字符')
      return
    }
    if (form.password.length < 8 || form.password.length > 64) {
      setError('密码长度需在 8-64 位之间')
      return
    }
    if (form.password !== form.confirm) {
      setError('两次输入的密码不一致')
      return
    }
    const wechat = form.wechat.trim()
    const qq = form.qq.trim()
    if (wechat && !WECHAT_PATTERN.test(wechat)) {
      setError('微信号不能包含空白字符，长度 1-64')
      return
    }
    if (qq && !QQ_PATTERN.test(qq)) {
      setError('QQ 号需为 5-20 位数字')
      return
    }

    setSubmitting(true)
    setError(null)
    try {
      const data = await registerUser({
        student_no: studentNo,
        password: form.password,
        ...(form.nickname.trim() ? { nickname: form.nickname.trim() } : {}),
        ...(wechat ? { wechat } : {}),
        ...(qq ? { qq } : {}),
      })
      setAuth(data)
      toast.success('注册成功，欢迎加入')
      navigate('/', { replace: true })
    } catch (err) {
      if (isApiError(err)) {
        setError(err.code === 'STUDENT_NO_EXISTS' ? '该学号已注册' : err.message)
      } else {
        setError('网络异常，请稍后重试')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AuthShell
      title="注册账号"
      subtitle="学号 + 密码即可加入，无需统一身份认证"
      footer={
        <>
          已有账号？{' '}
          <button
            className="font-medium text-accent underline underline-offset-4"
            type="button"
            onClick={() => navigate('/login')}
          >
            去登录
          </button>
        </>
      }
    >
      <form className="mx-auto flex w-full max-w-sm flex-col gap-4" onSubmit={submit}>
        <TextField
          className="w-full"
          name="student_no"
          value={form.student_no}
          onChange={update('student_no')}
        >
          <Label>学号 *</Label>
          <Input placeholder="4-32 位字母/数字/下划线/连字符" />
        </TextField>

        <div className="grid grid-cols-2 gap-4">
          <TextField
            className="w-full"
            name="password"
            value={form.password}
            onChange={update('password')}
          >
            <Label>密码 *</Label>
            <Input autoComplete="new-password" placeholder="8-64 位" type="password" />
          </TextField>
          <TextField
            className="w-full"
            name="confirm"
            value={form.confirm}
            onChange={update('confirm')}
          >
            <Label>确认密码 *</Label>
            <Input autoComplete="new-password" placeholder="再次输入" type="password" />
          </TextField>
        </div>

        <TextField
          className="w-full"
          name="nickname"
          value={form.nickname}
          onChange={update('nickname')}
        >
          <Label>昵称</Label>
          <Input placeholder="不填默认为“校园用户”" />
        </TextField>

        <div className="grid grid-cols-2 gap-4">
          <TextField
            className="w-full"
            name="wechat"
            value={form.wechat}
            onChange={update('wechat')}
          >
            <Label>微信</Label>
            <Input placeholder="选填，买卖联系用" />
          </TextField>
          <TextField className="w-full" name="qq" value={form.qq} onChange={update('qq')}>
            <Label>QQ</Label>
            <Input inputMode="numeric" placeholder="选填，5-20 位数字" />
          </TextField>
        </div>

        {error ? <p className="text-sm text-danger">{error}</p> : null}

        <Button className="w-full" isDisabled={submitting} isPending={submitting} type="submit">
          注册
        </Button>

        <p className="text-center text-xs text-muted">
          发布商品前需在“个人资料”中至少填写微信或 QQ 之一
        </p>
      </form>
    </AuthShell>
  )
}
