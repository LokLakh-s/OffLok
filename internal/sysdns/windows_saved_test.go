package sysdns

import "testing"

func TestSavedServers(t *testing.T) {
	for in, want := range map[string]string{
		"":                               "",
		"127.0.0.1":                      "",
		"::1":                            "",
		"127.0.0.1,192.168.1.1":          "192.168.1.1",
		"192.168.1.1 9.9.9.9":            "192.168.1.1,9.9.9.9",
		"fe80::1,2001:db8::53":           "2001:db8::53",
		"fe80::1%12":                     "",
		"0.0.0.0,not-an-address,1.1.1.1": "1.1.1.1",
	} {
		if got := savedServers(in); got != want {
			t.Errorf("savedServers(%q) = %q, want %q", in, got, want)
		}
	}
}
