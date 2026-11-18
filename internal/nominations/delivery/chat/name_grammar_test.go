package chat

import "testing"

func TestNameGrammarSplitsTheNameFromThePitch(t *testing.T) {
	cases := []struct{ body, name, pitch string }{
		{"Juno Vale. She rules", "Juno Vale", "She rules"},
		{"juno vale", "Juno Vale", ""},
		{"nominate juno vale. pitch", "Juno Vale", "pitch"},
		{"NOMINATE   remy quill: why", "Remy Quill", "why"},
		{"\"juno vale\" - pitch", "Juno Vale", "pitch"},
		{"“remy quill”pitch", "Remy Quill", "pitch"},
		{"j.k. vale. Pitch", "J.K. Vale", "Pitch"},
		{"Dr. juno vale. pitch", "Dr. Juno Vale", "pitch"},
		{"mr. t. pitch", "Mr. T. Pitch", ""},
		{"remy quill jr. Pitch", "Remy Quill Jr.", "Pitch"},
		{"jr. remy. pitch", "Jr", "remy. pitch"},
		{"juno vale...", "Juno Vale..", ""},
		{"juno vale.", "Juno Vale", ""},
		{"juno vale - she - rules", "Juno Vale", "she - rules"},
		{"juno vale: a: b", "Juno Vale", "a: b"},
		{"juno vale?! really", "Juno Vale", "really"},
		{"a.b.c. vale. pitch", "A.B.C. Vale", "pitch"},
		{"(dr. juno) vale. p", "(dr. Juno) Vale", "p"},
		{"JUNO VALE", "Juno Vale", ""},
		{"JUNO", "Juno", ""},
		{"juno-vale o'quill. p", "Juno-Vale O'quill", "p"},
		{"DeVale mcquill", "DeVale Mcquill", ""},
		{"juno vale 👀. pitch", "Juno Vale 👀", "pitch"},
		{"juno vale\nline two\n  line three ", "Juno Vale", "line two\n  line three"},
		{"juno vale. inline\nmore", "Juno Vale", "inline more"},
		{"st. juno of vale. p", "St. Juno Of Vale", "p"},
		{"q. vale", "Q. Vale", ""},
		{"x", "X", ""},
		{"juno  \t vale  pitch without split", "Juno Vale Pitch Without Split", ""},
		{"U.S.A. team. pitch", "U.S.A. Team", "pitch"},
		{"juno vale!pitch", "Juno Vale!pitch", ""},
		{"ÉLODIE VALE. p", "Élodie Vale", "p"},
	}
	for _, c := range cases {
		raw, pitch := splitNameAndPitch(c.body)
		if name := formatName(raw); name != c.name || pitch != c.pitch {
			t.Fatalf("B23 to B30 %q: want %q and %q, got %q and %q", c.body, c.name, c.pitch, name, pitch)
		}
	}
}
