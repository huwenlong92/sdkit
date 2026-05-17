package redisx

import "testing"

func TestClientKey(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		parts  []string
		want   string
	}{
		{name: "prefix and parts", prefix: "app", parts: []string{"user", "1"}, want: "app:user:1"},
		{name: "trim separators", prefix: "app:", parts: []string{":user:", "1"}, want: "app:user:1"},
		{name: "without prefix", parts: []string{"user", "1"}, want: "user:1"},
		{name: "skip empty parts", prefix: "app", parts: []string{"", "user"}, want: "app:user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{Prefix: tt.prefix}
			if got := c.Key(tt.parts...); got != tt.want {
				t.Fatalf("Key() = %q, want %q", got, tt.want)
			}
		})
	}
}
