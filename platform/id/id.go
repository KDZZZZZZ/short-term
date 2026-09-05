// Package id 生成各服务使用的不透明公开标识。
//
// docs/software-design.md 要求公开 ID 使用不透明字符串，绝不泄露数据库类型或结构。
// 这里采用 ULID 编码：48 位毫秒时间戳后接 80 位随机数，并以 Crockford base32
// 表示。客户端必须将其视为不透明值，但词法顺序与创建顺序一致，使仓储可以仅凭
// 主键分页。
package id

import (
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"
)

// crockford 是 Crockford base32 字母表，不包含 I、L、O 或 U。
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// encodedLen 是固定的 ULID 长度：每个字符 5 位，共表示 128 位。
const encodedLen = 26

// ErrInvalid 表示值不是格式正确的带前缀标识。
var ErrInvalid = errors.New("id: malformed identifier")

// Prefix 表示标识所属的聚合。它对客户端只是外观信息，但能让日志和追踪更易读。
type Prefix string

// 本仓库各服务拥有的聚合前缀。
const (
	PrefixAccount      Prefix = "u"
	PrefixProduct      Prefix = "p"
	PrefixProductImage Prefix = "img"
	PrefixTrade        Prefix = "t"
	PrefixReview       Prefix = "rv"
	PrefixConversation Prefix = "c"
	PrefixMessage      Prefix = "m"
	PrefixEvent        Prefix = "evt"
)

// Generator 生成单调递增的标识。单个 Generator 可安全并发使用。
type Generator struct {
	now func() time.Time

	mu       sync.Mutex
	lastMS   uint64
	lastRand [10]byte
}

// NewGenerator 使用指定时钟构造 Generator。时钟为 nil 时使用 time.Now。
func NewGenerator(now func() time.Time) *Generator {
	if now == nil {
		now = time.Now
	}
	return &Generator{now: now}
}

// New 为 prefix 指定的聚合返回新标识。
func (g *Generator) New(prefix Prefix) string {
	var raw [16]byte
	ms := uint64(g.now().UTC().UnixMilli())

	g.mu.Lock()
	if ms > g.lastMS {
		g.lastMS = ms
		fillRandom(&g.lastRand)
	} else {
		// 时钟刻度相同或回拨：递增随机部分，使同一毫秒内生成的标识保持创建顺序。
		ms = g.lastMS
		increment(&g.lastRand)
	}
	entropy := g.lastRand
	g.mu.Unlock()

	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	copy(raw[6:], entropy[:])

	return string(prefix) + "_" + encode(raw)
}

// Parse 校验 value 是否带有预期前缀的标识，并返回其编码主体。
// 这样传输层可以在接触数据库前拒绝明显伪造的标识。
func Parse(prefix Prefix, value string) (string, error) {
	head := string(prefix) + "_"
	body, found := strings.CutPrefix(value, head)
	if !found || len(body) != encodedLen {
		return "", ErrInvalid
	}
	for i := range len(body) {
		if strings.IndexByte(crockford, body[i]) < 0 {
			return "", ErrInvalid
		}
	}
	return body, nil
}

// encode 将 128 位数据转换为规范的 26 字符 Crockford base32 ULID 字符串。
// 该值在 130 位编码空间中右对齐，因此首字符不会超过 7，词法顺序与数值顺序一致。
func encode(raw [16]byte) string {
	out := make([]byte, encodedLen)
	for i := encodedLen - 1; i >= 0; i-- {
		out[i] = crockford[raw[15]&0x1f]
		shiftRight5(&raw)
	}
	return string(out)
}

// shiftRight5 在原地将大端序 128 位值除以 32。
func shiftRight5(raw *[16]byte) {
	var carry byte
	for i := range len(raw) {
		next := raw[i] & 0x1f
		raw[i] = carry<<3 | raw[i]>>5
		carry = next
	}
}

// fillRandom 在 crypto/rand 失败时触发 panic：静默退化为可预测值的标识生成器比崩溃更糟。
func fillRandom(dst *[10]byte) {
	if _, err := rand.Read(dst[:]); err != nil {
		panic("id: crypto/rand unavailable: " + err.Error())
	}
}

// increment 以大端序将熵加一。单个毫秒内发生溢出的概率极低；
// 发生时循环回绕，而不是阻塞。
func increment(dst *[10]byte) {
	for i := len(dst) - 1; i >= 0; i-- {
		dst[i]++
		if dst[i] != 0 {
			return
		}
	}
}
