package chat

import (
	"math"
	"testing"

	"gentalent/internal/nominations/domain"
)

func TestSimilarityIsTheRatcliffObershelpRatio(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"jordanabrewstr", "jordanabrewster", 0.9655172413793104},
		{"junovael", "junovale", 0.875},
		{"abcdxyz", "xyzabcd", 0.5714285714285714},
		{"aaaabbbb", "abababab", 0.625},
		{"remyquil", "remyquill", 0.9411764705882353},
		{"totallyunrelated", "junovale", 0.3333333333333333},
		{"ababababababababa", "bababababa", 0.7407407407407407},
	}
	for _, c := range cases {
		if got := similarity(c.a, c.b); math.Abs(got-c.want) > 1e-12 {
			t.Fatalf("B111 the similarity of %q and %q is %v, got %v", c.a, c.b, c.want, got)
		}
	}
}

func TestPersonKeyFoldsANameToItsLettersAndDigits(t *testing.T) {
	cases := map[string]domain.PersonID{
		"ﬁona Ｖale²":     "fionavale2",
		"José Vale":     "josévale",
		"STRASSE Straße": "strassestrasse",
		"Juno Vale 👀":    "junovale",
		"  Ñandú-12 ":    "ñandú12",
		"ǅemal":          "džemal",
	}
	for name, want := range cases {
		if got := personKey(name); got != want {
			t.Fatalf("B34 the key of %q is %q, got %q", name, want, got)
		}
	}
}
