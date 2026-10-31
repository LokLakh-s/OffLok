//go:build !windows && !darwin

package main

import "errors"

// notRunningKey is the tray's tooltip when offlokd does not answer: how to start it here.
const notRunningKey = "err.not-running"

var errCancelled = errors.New("cancelled")

// elevate is Windows's and macOS's: on Linux, offlokd has polkit ask for the password itself.
func elevate(string, string) error { return errors.ErrUnsupported }
