/**
 * Transitions.dev「Number pop-in」动效（MIT 展示样例的重实现）：
 * 数字逐位弹入 + 模糊 + 依次延迟。key 随 value 变化触发重挂载，动画随之重放。
 */
export function AnimatedDigits({ value, className }: { value: string; className?: string }) {
  return (
    <span key={value} className={`t-digit-group is-animating ${className ?? ''}`}>
      {Array.from(value).map((char, index) => (
        <span
          aria-hidden="true"
          className="t-digit"
          key={`${index}-${char}`}
          style={{ animationDelay: `${index * 70}ms` }}
        >
          {char}
        </span>
      ))}
      {/* 无障碍：完整文本给读屏，视觉层 aria-hidden */}
      <span className="sr-only">{value}</span>
    </span>
  )
}
