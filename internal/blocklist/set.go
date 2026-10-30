// Package blocklist turns block lists into compact sets of domain names and answers whether a name is blocked.
package blocklist

import (
	"slices"
	"strings"
)

// Set is a sorted, deduplicated set of lowercase domain names without the final dot, packed into one buffer: about
// 28 bytes per name, where a Go map would take about 90. Lookups are binary searches.
type Set struct {
	buf  []byte
	ends []uint32 // ends[i] is where name i stops in buf; it starts where name i-1 stopped
}

// NewSet builds a set from names (any order, duplicates allowed). The slice is sorted in place.
func NewSet(names []string) *Set {
	slices.Sort(names)
	names = slices.Compact(names)
	s := &Set{ends: make([]uint32, 0, len(names))}
	n := 0
	for _, name := range names {
		n += len(name)
	}
	s.buf = make([]byte, 0, n)
	for _, name := range names {
		s.buf = append(s.buf, name...)
		s.ends = append(s.ends, uint32(len(s.buf)))
	}
	return s
}

// Len is the number of names in the set.
func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return len(s.ends)
}

func (s *Set) at(i int) []byte {
	start := uint32(0)
	if i > 0 {
		start = s.ends[i-1]
	}
	return s.buf[start:s.ends[i]]
}

// compare orders b and s byte by byte, as strings.Compare would, without turning b into a string.
func compare(b []byte, s string) int {
	for i := 0; i < len(b) && i < len(s); i++ {
		if b[i] != s[i] {
			if b[i] < s[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(b) < len(s):
		return -1
	case len(b) > len(s):
		return 1
	}
	return 0
}

// Has reports whether name itself is in the set.
func (s *Set) Has(name string) bool {
	if s.Len() == 0 {
		return false
	}
	lo, hi := 0, len(s.ends)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		switch c := compare(s.at(mid), name); {
		case c == 0:
			return true
		case c < 0:
			lo = mid + 1
		default:
			hi = mid
		}
	}
	return false
}

// Covers returns the name in the set that covers name, itself or one of its parent domains (a list entry blocks a
// domain and everything under it), and "" when none does. Top-level domains alone never count.
func (s *Set) Covers(name string) string {
	for n := name; strings.Contains(n, "."); {
		if s.Has(n) {
			return n
		}
		_, n, _ = strings.Cut(n, ".")
	}
	return ""
}
