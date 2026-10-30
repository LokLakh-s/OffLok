//go:build !darwin

package unbound

// killStale has nothing to do: Linux's Pdeathsig and Windows's job object end Unbound with offlokd.
func killStale(string, string) {}
