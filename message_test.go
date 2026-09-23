package main

import "testing"

func TestIsCensoredIdent(t *testing.T) {
	tests := []struct {
		name  string
		ident string
		want  bool
	}{
		{"exact lowercase", "anonymous", true},
		{"title case", "Anonymous", true},
		{"uppercase", "ANONYMOUS", true},
		{"mixed case", "AnOnYmOuS", true},
		{"tilde prefix (no identd)", "~anonymous", true},
		{"tilde and mixed case", "~AnOnYmOuS", true},
		{"tilde only", "~", false},
		{"other tilde ident", "~birduser", false},
		{"other ident", "birduser", false},
		{"empty ident", "", false},
		{"superstring", "anonymous_troll", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCensoredIdent(tt.ident); got != tt.want {
				t.Errorf("isCensoredIdent(%q) = %v, want %v", tt.ident, got, tt.want)
			}
		})
	}
}
