package i18n

import "testing"

func TestLang(t *testing.T) {
	for env, want := range map[string]string{"fr_FR.UTF-8": "fr", "fr-FR": "fr", "de_DE.UTF-8": "en", "C": "en"} {
		t.Setenv("LC_ALL", env)
		if got := Lang(); got != want {
			t.Errorf("LC_ALL=%s: %s, want %s", env, got, want)
		}
	}
}

// Every message exists in both languages: a key missing from one catalog shows here.
func TestCatalogs(t *testing.T) {
	for k := range en {
		if _, ok := fr[k]; !ok {
			t.Errorf("%s: no French", k)
		}
	}
	for k := range fr {
		if _, ok := en[k]; !ok {
			t.Errorf("%s: no English", k)
		}
	}
}
