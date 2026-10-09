package jsc

import "testing"

func TestAvailable(t *testing.T) {
	if !Available() {
		t.Skip("node not available")
	}
	t.Logf("node: %s (%s)", nodePath, nodeVersion)
}

func TestSolveBadPlayer(t *testing.T) {
	if !Available() {
		t.Skip("node not available")
	}
	_, err := SolveN("var x = 1;", []string{"fake-n-param"})
	if err == nil {
		t.Fatal("expected solver error for invalid player, got nil")
	}
	t.Logf("got expected error: %v", err)
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		ver          string
		maj, min, pt int
		want         bool
	}{
		{"v24.18.0", 23, 5, 0, true},
		{"v23.5.0", 23, 5, 0, true},
		{"v23.4.9", 23, 5, 0, false},
		{"v22.0.0", 23, 5, 0, false},
	}
	for _, c := range cases {
		if got := versionAtLeast(c.ver, c.maj, c.min, c.pt); got != c.want {
			t.Errorf("versionAtLeast(%q, %d,%d,%d) = %v, want %v", c.ver, c.maj, c.min, c.pt, got, c.want)
		}
	}
}
