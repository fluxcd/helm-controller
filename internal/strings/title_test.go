package strings

import (
	"testing"

	"golang.org/x/text/language"
)

func TestTitle(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{
			name: "sentence",
			s:    "the quick brown fox jumps over the lazy dog",
			want: "The Quick Brown Fox Jumps Over The Lazy Dog",
		},
		{
			name: "sentence with uppercase word",
			s:    "the quick brown fox jumps over the LAZY dog",
			want: "The Quick Brown Fox Jumps Over The LAZY Dog",
		},
		{
			name: "empty string",
			s:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Title(tt.s); got != tt.want {
				t.Errorf("Title() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTitleWithLanguage(t *testing.T) {
	tests := []struct {
		name string
		s    string
		lang language.Tag
		want string
	}{
		{
			name: "Dutch sentence",
			s:    "de snelle bruine vos springt over de luie hond in ijburg",
			lang: language.Dutch,
			want: "De Snelle Bruine Vos Springt Over De Luie Hond In IJburg",
		},
		{
			name: "English sentence",
			s:    "the quick brown fox jumps over the lazy dog",
			lang: language.English,
			want: "The Quick Brown Fox Jumps Over The Lazy Dog",
		},
		{
			name: "English sentence with uppercase word",
			s:    "the quick brown fox jumps over the LAZY dog",
			lang: language.English,
			want: "The Quick Brown Fox Jumps Over The LAZY Dog",
		},
		{
			name: "empty",
			s:    "",
			lang: language.English,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TitleWithLanguage(tt.s, tt.lang); got != tt.want {
				t.Errorf("TitleWithLanguage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{
			name: "sentence",
			s:    "the quick brown fox jumps over the lazy dog",
			want: "The quick brown fox jumps over the lazy dog",
		},
		{
			name: "sentence with uppercase word",
			s:    "MacDonald had a farm",
			want: "MacDonald had a farm",
		},
		{
			name: "empty string",
			s:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.s); got != tt.want {
				t.Errorf("Normalize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeWithLanguage(t *testing.T) {
	tests := []struct {
		name string
		s    string
		lang language.Tag
		want string
	}{
		{
			name: "Dutch sentence",
			s:    "ijburg is een wijk in Amsterdam",
			lang: language.Dutch,
			want: "IJburg is een wijk in Amsterdam",
		},
		{
			name: "English sentence",
			s:    "the quick brown fox jumps over the lazy dog",
			lang: language.English,
			want: "The quick brown fox jumps over the lazy dog",
		},
		{
			name: "English sentence with uppercase word",
			s:    "MacDonald had a farm",
			lang: language.English,
			want: "MacDonald had a farm",
		},
		{
			name: "empty",
			s:    "",
			lang: language.Und,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeWithLanguage(tt.s, tt.lang); got != tt.want {
				t.Errorf("NormalizeWithLanguage() = %v, want %v", got, tt.want)
			}
		})
	}
}
