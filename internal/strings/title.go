package strings

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Title returns a copy of the string s with all Unicode letters that begin
// words mapped to their title case. It uses language.Und for word boundaries.
// For a more general solution, see TitleWithLanguage.
func Title(s string) string {
	return TitleWithLanguage(s, language.Und)
}

// TitleWithLanguage returns a copy of the string s with all Unicode letters
// that begin words mapped to their title case.
func TitleWithLanguage(s string, lang language.Tag) string {
	return cases.Title(lang, cases.NoLower).String(s)
}

// Normalize returns a copy of the string s with the first word mapped to its
// title case. It uses language.Und for word boundaries.
// For a more general solution, see NormalizeWithLanguage.
func Normalize(s string) string {
	return NormalizeWithLanguage(s, language.Und)
}

// NormalizeWithLanguage returns a copy of the string s with the first word
// mapped to its title case. If lang is not nil, it is used to determine the
// language for which the case transformation should be performed. If lang is
// nil, language.Und is used.
func NormalizeWithLanguage(s string, lang language.Tag) string {
	words := strings.Fields(s)
	if len(words) > 0 {
		words[0] = TitleWithLanguage(words[0], lang)
	}
	return strings.Join(words, " ")
}
