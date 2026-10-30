package i18n

import "testing"

func TestFirstAppleLanguage(t *testing.T) {
	if got := firstAppleLanguage("(\n    \"fr-FR\",\n    \"en-US\"\n)\n"); got != "fr-FR" {
		t.Fatalf("got %q, want fr-FR", got)
	}
	if got := firstAppleLanguage(""); got != "" {
		t.Fatalf("got %q for nothing", got)
	}
}
