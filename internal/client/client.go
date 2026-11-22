// Package client locates, downloads and launches the Flet client
// (the Flutter app that renders the UI).
package client

import (
	"os"
	"os/exec"
)

// EnvPath is the environment variable pointing to an installed Flet client.
// It can point to the folder, to Flet.app or directly to the executable.
// When unset, the client is downloaded and cached.
const EnvPath = "FLET_CLIENT_PATH"

// Start launches the client pointing it to the Go server address.
func Start(url string, debug bool) (*exec.Cmd, error) {
	path, err := Resolve()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, url)
	if debug {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
