package bogdanfinn

import "testing"

// 期望值取自真实 Chrome 发出的 sec-ch-ua.
func TestSecChUA(t *testing.T) {
	cases := map[int]string{
		120: `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`,
		124: `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`,
		149: `"Google Chrome";v="149", "Chromium";v="149", "Not)A;Brand";v="24"`,
	}
	for m, want := range cases {
		if got := secChUA(m); got != want {
			t.Errorf("secChUA(%d) = %s, want %s", m, got, want)
		}
	}
}
