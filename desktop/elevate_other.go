//go:build !windows

package main

import "errors"

var errCancelled = errors.New("cancelled")

// elevate is Windows's: elsewhere, offlokd has polkit (Linux) ask for the password itself.
func elevate(string, string) error { return errors.ErrUnsupported }
