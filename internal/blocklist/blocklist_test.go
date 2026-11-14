package blocklist

import (
	"slices"
	"strings"
	"testing"
)

func TestSet(t *testing.T) {
	s := NewSet([]string{"ads.example.org", "tracker.net", "tracker.net", "b.co"})
	if s.Len() != 3 {
		t.Fatalf("Len = %d", s.Len())
	}
	for name, want := range map[string]string{
		"ads.example.org": "ads.example.org", "x.y.ads.example.org": "ads.example.org", "example.org": "",
		"tracker.net": "tracker.net", "cdn.tracker.net": "tracker.net", "nottracker.net": "", "b.co": "b.co",
		"net": "", "": "",
	} {
		if got := s.Covers(name); got != want {
			t.Errorf("Covers(%q) = %q, want %q", name, got, want)
		}
	}
	var empty *Set
	if empty.Covers("ads.example.org") != "" || NewSet(nil).Covers("a.b") != "" {
		t.Error("an empty set covers nothing")
	}
}

func TestParse(t *testing.T) {
	list := `! Title: test
[Adblock Plus 2.0]
||ads.example.org^
||Tracker.NET^$important
||only-some.org^$client=192.168.1.2
@@||good.ads.example.org^
/regex.*/
.partial.com^
# a hosts part
0.0.0.0 hosts-one.com hosts-two.com
127.0.0.1 localhost
::1 ip6-localhost
plain.org
*.wild.org
not a domain
bad..name
`
	p, err := Parse(strings.NewReader(list))
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(p.Block)
	want := []string{"ads.example.org", "hosts-one.com", "hosts-two.com", "plain.org", "tracker.net", "wild.org"}
	if !slices.Equal(p.Block, want) {
		t.Errorf("Block = %v, want %v", p.Block, want)
	}
	if !slices.Equal(p.Allow, []string{"good.ads.example.org"}) {
		t.Errorf("Allow = %v", p.Allow)
	}
	if p.Skipped != 5 { // the $client rule, the regex, .partial.com^, "not a domain", bad..name
		t.Errorf("Skipped = %d", p.Skipped)
	}
}

func TestValidName(t *testing.T) {
	for _, n := range []string{"a.b", "xn--bcher-kva.example", "_dmarc.example.org"} {
		if !ValidName(n) {
			t.Errorf("%q refused", n)
		}
	}
	for _, n := range []string{"", "com", "a..b", "-a.b", "a b.c", "a.b.", strings.Repeat("a", 64) + ".com"} {
		if ValidName(n) {
			t.Errorf("%q accepted", n)
		}
	}
}

// The forms the AdGuard DNS filter uses for most of its exceptions, and $badfilter (checked on its 14/11/2026
// version: 205 of its 216 exceptions were skipped before).
func TestParseAdGuardForms(t *testing.T) {
	list := `||unsub.example.com^
@@||unsub.example.com^|
||api.example.net^
@@|api.example.net^|
||cancelled.example^
||cancelled.example^$badfilter
@@||kept-blocked.example^
@@||kept-blocked.example^$badfilter
||kept-blocked.example^
|exact.example.org^
||no-caret.example
`
	p, err := Parse(strings.NewReader(list))
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(p.Block)
	slices.Sort(p.Allow)
	if want := []string{"api.example.net", "exact.example.org", "kept-blocked.example", "unsub.example.com"}; !slices.Equal(p.Block, want) {
		t.Errorf("Block = %v, want %v", p.Block, want)
	}
	if want := []string{"api.example.net", "unsub.example.com"}; !slices.Equal(p.Allow, want) {
		t.Errorf("Allow = %v, want %v", p.Allow, want)
	}
	if p.Skipped != 1 { // ||no-caret.example
		t.Errorf("Skipped = %d", p.Skipped)
	}
}

func TestExceptionsNeverLiftMalicious(t *testing.T) {
	s := &Store{}
	s.active = []loaded{
		{Selected: Selected{Category: Ads, Source: Source{Name: "ads"}}, block: NewSet([]string{"cdn.example"}),
			allow: NewSet([]string{"site.example", "unbreak.cdn.example"})},
		{Selected: Selected{Category: Trackers, Source: Source{Name: "trackers"}}, block: NewSet([]string{"site.example"}), allow: NewSet(nil)},
		{Selected: Selected{Category: Malicious, Source: Source{Name: "phishing"}}, block: NewSet([]string{"login.site.example"}),
			allow: NewSet([]string{"safe.login.site.example"})},
	}
	for name, want := range map[string]string{
		"cdn.example":             "ads",
		"unbreak.cdn.example":     "", // the ad list's own exception
		"www.site.example":        "", // an ad list's exception unbreaks a tracker list's block, as AdGuard does
		"login.site.example":      "phishing",
		"x.login.site.example":    "phishing", // but never a malicious site's
		"safe.login.site.example": "",         // a list of malicious sites may
	} {
		m, ok := s.Lookup(name)
		if got := map[bool]string{true: m.Source}[ok]; got != want {
			t.Errorf("Lookup(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestSafeURL(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://lists.example.org/a.txt": true, "HTTPS://lists.example.org/a.txt": true,
		"http://127.0.0.1:8099/l.txt": true, "http://[::1]/l.txt": true, "http://localhost/l.txt": true,
		"http://lists.example.org/a.txt": false, "http://192.168.1.10/l.txt": false, "ftp://x.example/l": false,
		"file:///etc/passwd": false, "https://": false, "not a url": false,
	} {
		if got := SafeURL(raw); got != want {
			t.Errorf("SafeURL(%q) = %v, want %v", raw, got, want)
		}
	}
}
