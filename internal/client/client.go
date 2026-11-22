// Package client localiza e abre o cliente Flet (o app Flutter que desenha a tela).
package client

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnvPath é a variável de ambiente com o caminho do cliente Flet.
// No macOS pode apontar para o Flet.app ou direto para o executável.
const EnvPath = "FLET_CLIENT_PATH"

// Start abre o cliente apontando para o endereço do servidor Go.
func Start(url string, debug bool) (*exec.Cmd, error) {
	path := os.Getenv(EnvPath)
	if path == "" {
		return nil, errors.New("defina " + EnvPath + " com o caminho do cliente Flet (ex.: /caminho/Flet.app)")
	}
	if strings.HasSuffix(path, ".app") {
		path = filepath.Join(path, "Contents", "MacOS", "Flet")
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
