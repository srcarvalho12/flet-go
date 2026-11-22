package flet

import (
	"errors"
	"io"
	"net"
	"os"

	"github.com/srcarvalho12/flet-go/internal/client"
)

// EnvDebug ativa o log das mensagens trocadas com o cliente.
const EnvDebug = "FLETGO_DEBUG"

// App abre o cliente Flet e roda main quando ele se conectar.
// Retorna quando a janela é fechada.
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
	// Se o cliente fechar antes de conectar, destrava o Accept.
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
		ln.Close()
	}()

	conn, err := ln.Accept()
	if err != nil {
		return errors.New("flet: o cliente fechou antes de conectar")
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
