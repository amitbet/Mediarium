//go:build !windows

package main

func runAsService() bool { return false }

func handleUserMode([]string) (bool, error) { return false, nil }
