package main

import (
	"context"
	"edge-proxy/internal/proxy"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	proxy := proxy.NewProxy(configPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("Shutting down proxy...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := proxy.Stop(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	if err := proxy.Start(); err != nil {
		log.Fatal("Proxy error: ", err)
	}
}
