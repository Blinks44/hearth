package ui

import (
	"slices"
	"testing"
)

func TestCompareTitles(t *testing.T) {
	tests := []struct {
		name   string
		locale string
		in     []string
		want   []string
	}{
		{
			name:   "english case insensitive",
			locale: "en",
			in:     []string{"Rust", "go", "Python", "Bun", "node"},
			want:   []string{"Bun", "go", "node", "Python", "Rust"},
		},
		{
			name:   "russian cyrillic before latin and yo near ye",
			locale: "ru",
			in:     []string{"Go", "Ядро", "Всё", "Все", "Ель", "Ёлка", "Антенна", "Rust"},
			want:   []string{"Антенна", "Все", "Всё", "Ёлка", "Ель", "Ядро", "Go", "Rust"},
		},
		{
			name:   "german and french diacritics",
			locale: "de",
			in:     []string{"Übung", "Alpha", "Änderung", "Öffnen", "Éclair", "Beta"},
			want:   []string{"Alpha", "Änderung", "Beta", "Éclair", "Öffnen", "Übung"},
		},
		{
			name:   "spanish tilde",
			locale: "es",
			in:     []string{"Ópera", "Ñandú", "Nube", "Árbol"},
			want:   []string{"Árbol", "Ñandú", "Nube", "Ópera"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := slices.Clone(tc.in)
			slices.SortStableFunc(got, func(x, y string) int {
				return compareTitles(tc.locale, x, y)
			})
			if !slices.Equal(got, tc.want) {
				t.Fatalf("compareTitles(%q): got %v, want %v", tc.locale, got, tc.want)
			}
		})
	}
}
