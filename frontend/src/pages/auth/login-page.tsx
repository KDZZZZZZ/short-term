import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Button, TextField, Input, Label, toast } from '@heroui/react'
import { loginUser } from '@/lib/api/auth'
import { isApiError } from '@/lib/http'
import { useAuthStore } from '@/stores/auth-store'
import { AuthShell } from '@/components/auth/auth-shell'

function expiredNotice(): string | null {
  return new URLSearchParams(window.location.search).get('expired') === '1'
    ? '登录状态已失效或过期，请重新登录'
    : null
}

export function LoginPage() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const setAuth = useAuthStore((state) => state.setAuth)

  const [studentNo, setStudentNo] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(expiredNotice())
  const [submitting, setSubmitting] = useState(false)

  const from = params.get('from') || '/'

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!studentNo.trim() || !password) {
      setError('请输入学号和密码')
      return
    }
    setSubmitting(true)
    setError(null)
    try {
      const data = await loginUser({ student_no: studentNo.trim(), password })
      setAuth(data)
      toast.success(`欢迎回来，${data.user.nickname}`)
      navigate(from, { replace: true })
    } catch (err) {
      if (isApiError(err)) {
        setError(err.status === 401 ? '学号或密码错误' : err.message)
      } else {
        setError('网络异常，请稍后重试')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AuthShell
      title="欢迎回来"
      subtitle="使用学号和密码登录校园二手集市"
      footer={
        <>
          还没有账号？{' '}
          <button
            className="font-medium text-accent underline underline-offset-4"
            type="button"
            onClick={() => navigate('/register')}
          >
            免费注册
          </button>
        </>
      }
    >
      <form className="mx-auto flex w-full max-w-sm flex-col gap-4" onSubmit={submit}>
        <TextField
          className="w-full"
          isInvalid={Boolean(error)}
          name="student_no"
          value={studentNo}
          onChange={setStudentNo}
        >
          <Label>学号</Label>
          <Input autoComplete="username" placeholder="请输入学号" />
        </TextField>

        <TextField
          className="w-full"
          isInvalid={Boolean(error)}
          name="password"
          value={password}
          onChange={setPassword}
        >
          <Label>密码</Label>
          <Input autoComplete="current-password" placeholder="请输入密码" type="password" />
        </TextField>

        {error ? <p className="text-sm text-danger">{error}</p> : null}

        <Button className="w-full" isDisabled={submitting} isPending={submitting} type="submit">
          登录
        </Button>
      </form>
    </AuthShell>
  )
}
