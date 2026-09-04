/**
 * KokonutUI「use-auto-resize-textarea」
 * 来源：https://kokonutui.com/r/use-auto-resize-textarea.json（MIT，按注册表声明使用）。
 * 聊天输入框随内容自动增高。
 */

import { useCallback, useEffect, useRef } from 'react'

interface UseAutoResizeTextareaProps {
  minHeight: number
  maxHeight?: number
}

export function useAutoResizeTextarea({ minHeight, maxHeight }: UseAutoResizeTextareaProps) {
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const adjustHeight = useCallback(
    (reset?: boolean) => {
      const textarea = textareaRef.current
      if (!textarea) return

      if (reset) {
        textarea.style.height = `${minHeight}px`
        return
      }

      // 先收缩到最小高度，再按 scrollHeight 重新撑开
      textarea.style.height = `${minHeight}px`

      const newHeight = Math.max(minHeight, Math.min(textarea.scrollHeight, maxHeight ?? Number.POSITIVE_INFINITY))

      textarea.style.height = `${newHeight}px`
    },
    [minHeight, maxHeight],
  )

  useEffect(() => {
    const textarea = textareaRef.current
    if (textarea) {
      textarea.style.height = `${minHeight}px`
    }
  }, [minHeight])

  useEffect(() => {
    const handleResize = () => adjustHeight()
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [adjustHeight])

  return { textareaRef, adjustHeight }
}
