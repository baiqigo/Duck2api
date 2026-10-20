package proxys

import (
	"testing"
	"time"
)

func TestGetProxyIPSkipsCooling(t *testing.T) {
	p := NewIProxyIP([]string{"a", "b", "c"})
	p.MarkBad("a", time.Minute)
	p.MarkBad("b", 2*time.Minute)
	for i := 0; i < 3; i++ {
		if got := p.GetProxyIP(); got != "c" {
			t.Fatalf("round %d: got %s, want c", i, got)
		}
	}
	p.MarkBad("c", 3*time.Minute)
	if got := p.GetProxyIP(); got != "a" { // 全冷却时取最早解冻
		t.Fatalf("all cooling: got %s, want a", got)
	}
}
