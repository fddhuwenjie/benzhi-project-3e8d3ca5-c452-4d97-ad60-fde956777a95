package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"siteclosure/internal/application"
	"siteclosure/internal/storage"
	webui "siteclosure/internal/web"
)

func main() {
	if err := run(); err != nil {
		log.Printf("启动失败: %v", err)
		os.Exit(1)
	}
}
func run() error {
	cfg, err := parseConfig()
	if err != nil {
		return err
	}
	dataDir := cfg.dataDir
	if cfg.selftest {
		dataDir, err = os.MkdirTemp("", "siteclosure-selftest-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dataDir)
	}
	store, err := storage.New(dataDir)
	if err != nil {
		return fmt.Errorf("恢复本地数据: %w", err)
	}
	app := application.New(store)
	handler := webui.New(app).Handler()
	listener, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.addr, err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		e := server.Serve(listener)
		if e != nil && !errors.Is(e, http.ErrServerClosed) {
			serveErr <- e
		}
		close(serveErr)
	}()
	if cfg.selftest {
		err = runSelftest("http://" + listener.Addr().String())
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(ctx)
		if err != nil {
			return err
		}
		return shutdownErr
	}
	log.Printf("探方回填封护验收台已监听 http://%s", listener.Addr())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
	case e := <-serveErr:
		if e != nil {
			return e
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}
