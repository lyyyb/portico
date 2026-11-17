package launcher

import "testing"

func TestLocalHost(t *testing.T) {
	allowed := []string{"127.0.0.1:18080", "localhost:18080", "[::1]:18080", "127.0.0.1"}
	denied := []string{"evil.com:18080", "192.168.1.10:18080", "127.0.0.1.evil.com:18080"}
	for _, h := range allowed {
		if !localHost(h) {
			t.Errorf("Host %q 应被允许", h)
		}
	}
	for _, h := range denied {
		if localHost(h) {
			t.Errorf("Host %q 应被拒绝", h)
		}
	}
}
