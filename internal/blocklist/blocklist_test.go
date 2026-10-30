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
