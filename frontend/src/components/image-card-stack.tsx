import { useEffect, useRef, useState } from 'react'
import { animate, easeIn, mix, motion, progress, useMotionValue, useTransform, wrap } from 'motion/react'

/**
 * 商品图片卡片堆（参照 examples.motion.dev/react/card-stack）：
 * 所有卡片绝对定位重叠，"顶层"由 currentIndex 的环形偏移决定；划走一张只是把
 * 指针前移并把该卡弹回原位——因为它已落到堆底，视觉上就是"飞到后面"。
 * 底层卡片的缩放/透明度由 zIndex 派生。
 */
export function ImageCardStack({
  images,
  initialIndex = 0,
  maxRotate = 5,
}: {
  images: { src: string; alt?: string }[]
  initialIndex?: number
  maxRotate?: number
}) {
  const [currentIndex, setCurrentIndex] = useState(() =>
    Math.min(Math.max(initialIndex, 0), images.length - 1),
  )
  const ref = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(400)

  useEffect(() => {
    if (!ref.current) return
    setWidth(ref.current.offsetWidth)
  }, [])

  return (
    <div ref={ref} className="relative h-full w-full">
      {images.map((image, index) => (
        <StackCard
          key={image.src}
          src={image.src}
          alt={image.alt}
          index={index}
          totalImages={images.length}
          currentIndex={currentIndex}
          maxRotate={maxRotate}
          minDistance={width * 0.5}
          setNextImage={() => setCurrentIndex(wrap(0, images.length, currentIndex + 1))}
        />
      ))}
    </div>
  )
}

function StackCard({
  src,
  alt,
  index,
  totalImages,
  currentIndex,
  maxRotate,
  setNextImage,
  minDistance = 200,
  minSpeed = 50,
}: {
  src: string
  alt?: string
  index: number
  totalImages: number
  currentIndex: number
  maxRotate: number
  setNextImage: () => void
  minDistance?: number
  minSpeed?: number
}) {
  // 用 Math.sin(index) 确定性地生成每张卡的基准倾斜，视觉上自然错落
  const baseRotation = mix(0, maxRotate, Math.sin(index))
  const x = useMotionValue(0)
  const rotate = useTransform(x, [0, 400], [baseRotation, baseRotation + 10], { clamp: false })
  const zIndex = totalImages - wrap(totalImages, 0, index - currentIndex + 1)

  const onDragEnd = () => {
    const distance = Math.abs(x.get())
    const speed = Math.abs(x.getVelocity())
    if (distance > minDistance || speed > minSpeed) {
      setNextImage()
      animate(x, 0, { type: 'spring', stiffness: 600, damping: 50 })
    } else {
      animate(x, 0, { type: 'spring', stiffness: 300, damping: 50 })
    }
  }

  const opacity = progress(totalImages * 0.25, totalImages * 0.75, zIndex)
  const depth = progress(0, totalImages - 1, zIndex)
  const scale = mix(0.5, 1, easeIn(depth))

  return (
    <motion.div
      className="absolute inset-0 overflow-hidden rounded-2xl bg-surface-secondary shadow-2xl will-change-transform"
      style={{ zIndex, rotate, x }}
      initial={{ opacity: 0, scale: 0.3 }}
      animate={{ opacity, scale }}
      whileTap={index === currentIndex ? { scale: 0.98 } : undefined}
      transition={{ type: 'spring', stiffness: 600, damping: 30 }}
      drag={index === currentIndex ? 'x' : false}
      onDragEnd={onDragEnd}
    >
      <img
        alt={alt}
        className="h-full w-full select-none object-cover"
        draggable={false}
        onPointerDown={(e) => e.preventDefault()}
        src={src}
      />
    </motion.div>
  )
}
