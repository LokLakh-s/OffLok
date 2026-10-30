package i18n

import (
	"os/exec"
	"strings"
)

// systemLang is the first of the user's preferred languages ("fr-FR"): an app opened from the Finder or the Dock
// gets no LANG, which only Terminal sets.
func systemLang() string {
	out, err := exec.Command("/usr/bin/defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return ""
	}
	return firstAppleLanguage(string(out))
}

// firstAppleLanguage reads the first entry of `defaults read -g AppleLanguages`: `(\n    "fr-FR",\n    "en-FR"\n)`.
func firstAppleLanguage(out string) string {
	for _, line := range strings.Split(out, "\n") {
		l := strings.Trim(strings.TrimSpace(line), `",`)
		if l != "" && l != "(" && l != ")" {
			return l
		}
	}
	return ""
}
