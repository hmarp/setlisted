package normalize

import "testing"

func TestName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Gojira", "gojira"},
		{"MOTÖRHEAD", "motorhead"},
		{"Motörhead", "motorhead"},
		{"Blue Öyster Cult", "blue oyster cult"},
		{"Beyoncé", "beyonce"},
		{"Mötley Crüe", "motley crue"}, // already decomposed
		{"  The Sword \t", "the sword"},
		{"Iron   Maiden", "iron maiden"},
		{"Sigur Rós", "sigur ros"},   // non-breaking space
		{"Ørkenkjøtt", "ørkenkjøtt"}, // ø doesn't decompose
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		if got := Name(tt.in); got != tt.want {
			t.Errorf("Name(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
