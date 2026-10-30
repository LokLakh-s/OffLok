//go:build !windows && !darwin

package main

import "errors"

var errCancelled = errors.New("cancelled")

// elevate is Windows's and macOS's: on Linux, offlokd has polkit ask for the password itself.
func elevate(string, string) error { return errors.ErrUnsupported }
