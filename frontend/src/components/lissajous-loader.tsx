import { useEffect, useRef } from 'react'

/**
 * Lissajous 曲线拖尾加载动画。
 * 改写自 math-curve-loaders 的「Lissajous Drift」(rAF 驱动粒子拖尾):
 * 按需求去掉了灰色完整路线,只保留粒子轨迹;颜色走 currentColor 跟随主题。
 * 系统开启「减少动态效果」时静态呈现一帧。
 */

const PARTICLE_COUNT = 68
const TRAIL_SPAN = 0.34
const DURATION_MS = 6000
const PULSE_DURATION_MS = 5400
const AMP = 24
const AMP_BOOST = 6
const FREQ_X = 3
const FREQ_Y = 4
const PHASE = 1.57
const Y_SCALE = 0.92

function getDetailScale(time: number): number {
  const pulseProgress = (time % PULSE_DURATION_MS) / PULSE_DURATION_MS
  const pulseAngle = pulseProgress * Math.PI * 2
  return 0.52 + ((Math.sin(pulseAngle + 0.55) + 1) / 2) * 0.48
}

function point(progress: number, detailScale: number): { x: number; y: number } {
  const t = progress * Math.PI * 2
  const amp = AMP + detailScale * AMP_BOOST
  return {
    x: 50 + Math.sin(FREQ_X * t + PHASE) * amp,
    y: 50 + Math.sin(FREQ_Y * t) * (amp * Y_SCALE),
  }
}

function getParticle(index: number, progress: number, detailScale: number) {
  const tailOffset = index / (PARTICLE_COUNT - 1)
  const current = point(((progress % 1) + 1) % 1 - tailOffset * TRAIL_SPAN, detailScale)
  const fade = Math.pow(1 - tailOffset, 0.56)
  return {
    x: current.x,
    y: current.y,
    radius: 0.9 + fade * 2.7,
    opacity: 0.04 + fade * 0.96,
  }
}

export function LissajousLoader({ className }: { className?: string }) {
  const groupRef = useRef<SVGGElement>(null)

  useEffect(() => {
    const group = groupRef.current
    if (!group) return
    const nodes = Array.from(group.children) as SVGCircleElement[]

    const applyFrame = (progress: number, time: number) => {
      const detailScale = getDetailScale(time)
      nodes.forEach((node, index) => {
        const particle = getParticle(index, progress, detailScale)
        node.setAttribute('cx', particle.x.toFixed(2))
        node.setAttribute('cy', particle.y.toFixed(2))
        node.setAttribute('r', particle.radius.toFixed(2))
        node.setAttribute('opacity', particle.opacity.toFixed(3))
      })
    }

    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      applyFrame(0, 0)
      return
    }

    const startedAt = performance.now()
    let raf = 0
    const render = (now: number) => {
      const time = now - startedAt
      applyFrame((time % DURATION_MS) / DURATION_MS, time)
      raf = requestAnimationFrame(render)
    }
    raf = requestAnimationFrame(render)
    return () => cancelAnimationFrame(raf)
  }, [])

  return (
    <svg aria-hidden="true" className={className} fill="none" viewBox="0 0 100 100">
      <g ref={groupRef}>
        {Array.from({ length: PARTICLE_COUNT }, (_, index) => (
          <circle key={index} fill="currentColor" opacity={0} />
        ))}
      </g>
    </svg>
  )
}
