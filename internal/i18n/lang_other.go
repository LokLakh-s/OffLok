//go:build !windows

package i18n

// systemLang: Linux and macOS set the locale's variables, which Lang reads first.
func systemLang() string { return "" }
