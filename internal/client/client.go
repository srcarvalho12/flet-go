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
// assetsDir, when not empty, is the folder with the app's own files.
func Start(url, assetsDir string, debug bool) (*exec.Cmd, error) {
	path, err := Resolve()
	if err != nil {
		return nil, err
	}
	args := []string{url}
	if assetsDir != "" {
		// The client takes [url, pid file, assets dir]. The pid file is not
		// needed, so it goes to the null device.
		args = append(args, os.DevNull, assetsDir)
	}
	cmd := exec.Command(path, args...)
	if debug {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
