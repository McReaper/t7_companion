package build

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// gameLanguage is the language the game at root plays in, and where that was
// read: localization.txt's first line (what the game reads at startup to
// pick its language), else the language Steam installed it in
// (steamapps/appmanifest_311210.acf, two folders up); "" when neither says.
func gameLanguage(root string) (lang, from string) {
	if b, err := os.ReadFile(filepath.Join(root, "localization.txt")); err == nil {
		first, _, _ := strings.Cut(strings.TrimLeft(string(b), "\ufeff\r\n\t "), "\n")
		if l := strings.ToLower(strings.TrimSpace(first)); languageName.MatchString(l) {
			return l, "localization.txt"
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "..", "..", "appmanifest_311210.acf")); err == nil {
		if m := steamLanguage.FindSubmatch(b); m != nil {
			return strings.ToLower(string(m[1])), "Steam's appmanifest_311210.acf"
		}
	}
	return "", ""
}

var (
	languageName  = regexp.MustCompile(`^[a-z]+$`)
	steamLanguage = regexp.MustCompile(`"language"\s+"([A-Za-z]+)"`)
)

// linkLanguage is the language the link passes and, when it wasn't asked for,
// why: the game's own, read from its install, else english. The game loads a
// map's <language>_<map>.ff in its own language and stops on "Could not find
// zone" without it.
func linkLanguage(asked, game string) (lang, note string) {
	if asked != "" {
		return asked, ""
	}
	if l, from := gameLanguage(game); l != "" {
		return l, "language " + l + ": the game's, from its " + from
	}
	return "english", "language english: the game's install names none (localization.txt, Steam's manifest)"
}
