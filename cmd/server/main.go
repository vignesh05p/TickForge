package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/vigneshprabhu/tickforge/internal/config"
	"github.com/vigneshprabhu/tickforge/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	app := server.New(cfg, nil)
	httpServer := app.HTTPServer()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("tickforge server listening on %s", cfg.HTTPAddr)
		errCh <- httpServer.ListenAndServe()
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-signalCh:
		log.Printf("received %s, shutting down", sig)
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown failed: %v", err)
	}
	log.Print("shutdown complete")
}
