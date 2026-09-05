import { useEffect } from 'react'
import { motion } from 'motion/react'
import { X } from 'lucide-react'
import { Button } from '@heroui/react'
import { ImageCardStack } from '@/components/image-card-stack'

/**
 * 商品图片灯箱：放大居中的卡片堆，左右拖动切换图片。
 * 点击空白处或按 Esc 关闭；不做移动端适配。
 */
export function ProductImageLightbox({
  urls,
  initialIndex,
  onClose,
}: {
  urls: string[]
  initialIndex: number
  onClose: () => void
}) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <motion.div
      animate={{ opacity: 1 }}
      className="fixed inset-0 z-50 flex flex-col items-center justify-center gap-5 bg-black/80 backdrop-blur-sm"
      exit={{ opacity: 0 }}
      initial={{ opacity: 0 }}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <Button
        isIconOnly
        aria-label="关闭"
        className="absolute right-5 top-5 bg-white/10 text-white hover:bg-white/20"
        onPress={onClose}
        size="sm"
        variant="tertiary"
      >
        <X className="size-4" />
      </Button>

      {/* 卡片堆容器；阻止拖拽/点击冒泡到背景导致误关 */}
      <div
        className="aspect-[3/4] h-[min(70vh,560px)] max-w-[92vw]"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <ImageCardStack
          images={urls.map((src, index) => ({ src, alt: `商品图片 ${index + 1}` }))}
          initialIndex={initialIndex}
        />
      </div>

      <p className="text-sm text-white/70">左右拖动查看其他图片 · 点击空白处或按 Esc 关闭</p>
    </motion.div>
  )
}
