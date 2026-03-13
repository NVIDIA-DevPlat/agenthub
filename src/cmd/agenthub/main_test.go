package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunVersion(t *testing.T) {
	err := run([]string{"version"})
	require.NoError(t, err)
}

func TestRunUnknownCommand(t *testing.T) {
	err := run([]string{"notacommand"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "notacommand")
}

func TestRunDefaultsToServe(t *testing.T) {
	// 'serve' requires a running config file and dolt server.
	// With a non-existent config path it should fail with a config error.
	t.Setenv("AGENTHUB_CONFIG", "/nonexistent/config.yaml")
	err := run([]string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading config")
}
