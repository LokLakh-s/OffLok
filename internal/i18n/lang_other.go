//go:build !windows && !darwin

package i18n

// systemLang: Linux sets the locale's variables, which Lang reads first.
func systemLang() string { return "" }
