package flet

import (
	"errors"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"

	"github.com/srcarvalho12/flet-go/internal/client"
)

// EnvDebug enables logging of the messages exchanged with the client.
const EnvDebug = "FLETGO_DEBUG"

// AppOptions configures a desktop app. All fields are optional.
type AppOptions struct {
	// AssetsDir is a folder with the app's own files (images, fonts,
	// icons): "assets/logo.png" is used as &flet.Image{Src: "logo.png"}.
	// Also set by FLET_ASSETS_DIR.
	AssetsDir string
}

// App launches the Flet client and runs main once it connects.
// It returns when the window is closed.
//
// If the connection drops while the window is open, the client reconnects
// and gets the same page back.
func App(main func(page *Page), opts ...AppOptions) error {
	debug := os.Getenv(EnvDebug) != ""
	var o AppOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if v := os.Getenv(EnvAssetsDir); v != "" {
		o.AssetsDir = v
	}
	if o.AssetsDir != "" {
		abs, err := filepath.Abs(o.AssetsDir)
		if err != nil {
			return err
		}
		o.AssetsDir = abs
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()

	cmd, err := client.Start("tcp://"+ln.Addr().String(), o.AssetsDir, debug)
	if err != nil {
		return err
	}
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()

	// Close the window when the program is interrupted.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	srv := newServer(main, debug)
	var connected atomic.Bool
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			connected.Store(true)
			go func() {
				defer conn.Close()
				err := srv.serve(socketTransport{conn}, clientInfo{})
				if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
					log.Printf("flet: connection error: %v", err)
				}
			}()
		}
	}()

	select {
	case <-exited:
	case <-sig:
		cmd.Process.Kill()
		<-exited
	}
	if !connected.Load() {
		return errors.New("flet: the client exited before connecting")
	}
	return nil
}
