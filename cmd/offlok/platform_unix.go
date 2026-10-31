//go:build !windows && !darwin

package main

const (
	deniedKey     = "err.denied"
	notRunningKey = "err.not-running"
	updateKey     = "update.available"
)
