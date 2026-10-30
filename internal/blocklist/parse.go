package blocklist

import (
	"bufio"
	"io"
	"strings"
)

// Parsed is what one list gives: the names it blocks and the names it allows again (AdBlock's @@ exceptions). Each
// name stands for the domain and everything under it.
type Parsed struct {
	Block   []string
	Allow   []string
	Skipped int // rules OffLok does not understand (regular expressions, modifiers that narrow a rule)
}

// Parse reads a list in the AdBlock DNS syntax (||example.org^, @@||example.org^), the hosts syntax
// (0.0.0.0 example.org) or one domain per line, and comments in any of them.
func Parse(r io.Reader) (Parsed, error) {
	var p Parsed
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	for s.Scan() {
		line := strings.ToLower(strings.TrimSpace(s.Text()))
		if line == "" || line[0] == '!' || line[0] == '#' || line[0] == '[' {
			continue
		}
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		switch {
		case strings.HasPrefix(line, "@@"):
			if name, ok := adblock(line[2:]); ok {
				p.Allow = append(p.Allow, name)
			} else {
				p.Skipped++
			}
		case strings.HasPrefix(line, "||"):
			if name, ok := adblock(line); ok {
				p.Block = append(p.Block, name)
			} else {
				p.Skipped++
			}
		default:
			names, ok := hostsOrPlain(line)
			if !ok {
				p.Skipped++
			}
			p.Block = append(p.Block, names...)
		}
	}
	return p, s.Err()
}

// adblock reads ||name^ with no modifier but $important (which only orders rules, irrelevant here).
func adblock(rule string) (string, bool) {
	rule, mods, _ := strings.Cut(rule, "$")
	if mods != "" && mods != "important" {
		return "", false
	}
	name, ok := strings.CutPrefix(rule, "||")
	if !ok {
		return "", false
	}
	name, ok = strings.CutSuffix(name, "^")
	if !ok || !ValidName(name) {
		return "", false
	}
	return name, true
}

var hostsAddrs = map[string]bool{"0.0.0.0": true, "127.0.0.1": true, "::": true, "::1": true, "0": true}

var hostsNoise = map[string]bool{"localhost": true, "localhost.localdomain": true, "local": true, "broadcasthost": true,
	"ip6-localhost": true, "ip6-loopback": true, "0.0.0.0": true}

func hostsOrPlain(line string) ([]string, bool) {
	f := strings.Fields(line)
	if hostsAddrs[f[0]] {
		var out []string
		for _, n := range f[1:] {
			if !hostsNoise[n] && ValidName(n) {
				out = append(out, n)
			}
		}
		return out, len(out) > 0 || len(f) == 2 && hostsNoise[f[1]]
	}
	name := strings.TrimPrefix(f[0], "*.")
	if len(f) == 1 && ValidName(name) {
		return []string{name}, true
	}
	return nil, false
}

// ValidName reports whether name is a plain domain of at least two labels (no final dot, lowercase).
func ValidName(name string) bool {
	if len(name) == 0 || len(name) > 253 || !strings.Contains(name, ".") {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	return true
}
