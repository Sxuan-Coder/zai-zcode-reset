package upstream

import (
	"regexp"
	"testing"
)

// 与 ZCode 客户端主路径 crypto.randomUUID() 一致：UUID v4，36 字符。
// 上游 use 接口对非 UUID 幂等键回 3001 parameter error。
var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewIdempotencyKeyIsUUIDV4(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		key := NewIdempotencyKey()
		if !uuidV4Pattern.MatchString(key) {
			t.Fatalf("幂等键不是 UUID v4: %q", key)
		}
		if len(key) > 64 {
			t.Fatalf("幂等键超过客户端 64 字符上限: %q", key)
		}
		if seen[key] {
			t.Fatalf("幂等键重复: %q", key)
		}
		seen[key] = true
	}
}
