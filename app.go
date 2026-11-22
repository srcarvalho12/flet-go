package flet

import (
	"errors"
	"io"
	"net"
	"os"

	"github.com/srcarvalho12/flet-go/internal/client"
)

// EnvDebug enables logging of the messages exchanged with the client.
const EnvDebug = "FLETGO_DEBUG"

// App launches the Flet client and runs main once it connects.
// It returns when the window is closed.
func App(main func(page *Page)) error {
	debug := os.Getenv(EnvDebug) != ""

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()

	cmd, err := client.Start("tcp://"+ln.Addr().String(), debug)
	if err != nil {
		return err
	}
	// If the client exits before connecting, unblock Accept.
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
		ln.Close()
	}()

	conn, err := ln.Accept()
	if err != nil {
		return errors.New("flet: the client exited before connecting")
	}
	defer conn.Close()

	s := &session{conn: conn, main: main, debug: debug}
	err = s.run()

	select {
	case <-exited:
	default:
		cmd.Process.Kill()
	}

	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}
