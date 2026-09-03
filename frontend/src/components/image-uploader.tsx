import { useRef, useState } from 'react'
import { Button, toast } from '@heroui/react'
import { ImagePlus, X } from 'lucide-react'
import {
  ALLOWED_IMAGE_TYPES,
  MAX_IMAGE_BYTES,
  MAX_IMAGES,
} from '@/lib/format'

interface ImageUploaderProps {
  /** 受控文件列表（含已存在的 server 图片时由调用方另行渲染）。 */
  files: File[]
  onChange: (files: File[]) => void
  /** 已有图片数量（编辑图片页补充上传时使用）。 */
  existingCount?: number
  max?: number
}

/** 最多三张、单张 5MiB、JPEG/PNG/WebP（与契约一致的前置校验，服务端仍以真实字节判定）。 */
export function ImageUploader({ files, onChange, existingCount = 0, max = MAX_IMAGES }: ImageUploaderProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [previews, setPreviews] = useState<Record<string, string>>({})

  const remaining = max - existingCount - files.length

  const addFiles = (incoming: FileList | null) => {
    if (!incoming?.length) return
    const accepted: File[] = []
    for (const file of Array.from(incoming)) {
      if (!ALLOWED_IMAGE_TYPES.has(file.type)) {
        toast.warning(`不支持的图片格式：${file.name}，仅支持 JPEG/PNG/WebP`)
        continue
      }
      if (file.size > MAX_IMAGE_BYTES) {
        toast.warning(`图片超过 5MiB 上限：${file.name}`)
        continue
      }
      accepted.push(file)
    }
    const room = max - existingCount - files.length
    if (accepted.length > room) {
      toast.warning(`最多还能上传 ${Math.max(room, 0)} 张图片`)
      accepted.length = Math.max(room, 0)
    }
    if (accepted.length === 0) return
    const nextPreviews: Record<string, string> = { ...previews }
    for (const file of accepted) {
      nextPreviews[file.name + file.size] = URL.createObjectURL(file)
    }
    setPreviews(nextPreviews)
    onChange([...files, ...accepted])
  }

  const removeAt = (index: number) => {
    const file = files[index]
    if (file) {
      const url = previews[file.name + file.size]
      if (url) URL.revokeObjectURL(url)
    }
    onChange(files.filter((_, i) => i !== index))
  }

  return (
    <div className="flex flex-wrap items-start gap-3">
      {files.map((file, index) => (
        <div
          className="group relative h-24 w-24 overflow-hidden rounded-xl border border-border-secondary bg-surface"
          key={`${file.name}-${file.size}-${index}`}
        >
          <img
            alt={file.name}
            className="h-full w-full object-cover"
            src={previews[file.name + file.size]}
          />
          <Button
            isIconOnly
            aria-label={`移除图片 ${file.name}`}
            className="absolute end-1 top-1 opacity-90"
            size="sm"
            variant="primary"
            onPress={() => removeAt(index)}
          >
            <X className="size-3.5" />
          </Button>
          {index === 0 && existingCount === 0 ? (
            <span className="absolute inset-x-0 bottom-0 bg-black/55 py-0.5 text-center text-[10px] text-white">
              封面
            </span>
          ) : null}
        </div>
      ))}

      {remaining > 0 ? (
        <>
          <input
            accept="image/jpeg,image/png,image/webp"
            className="hidden"
            multiple
            ref={inputRef}
            type="file"
            onChange={(event) => {
              addFiles(event.target.files)
              event.target.value = ''
            }}
          />
          <Button
            isIconOnly
            aria-label="添加图片"
            className="h-24 w-24 border border-dashed border-border-tertiary text-muted"
            size="lg"
            variant="tertiary"
            onPress={() => inputRef.current?.click()}
          >
            <ImagePlus className="size-6" />
          </Button>
        </>
      ) : null}

      <p className="w-full text-xs text-muted">
        最多 {max} 张，单张不超过 5MiB，支持 JPEG/PNG/WebP；第一张将作为封面。
      </p>
    </div>
  )
}
