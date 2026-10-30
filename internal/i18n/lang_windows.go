package i18n

import "golang.org/x/sys/windows"

// systemLang is the user's display language, first of their preferred ones ("fr-FR").
func systemLang() string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(langs) == 0 {
		return ""
	}
	return langs[0]
}
