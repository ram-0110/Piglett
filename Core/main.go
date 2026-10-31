package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"piglett/core/internal/farm"
)

const version = "0.2.0"

func main() {
	showVersion := flag.Bool("version", false, "print the Core version")
	dataDir := flag.String("data-dir", "", "Electron UserData directory")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *dataDir == "" {
		fmt.Fprintln(os.Stderr, "Core requires --data-dir")
		os.Exit(2)
	}
	absolute, err := filepath.Abs(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	server, err := farm.NewServer(absolute)
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	ready := map[string]any{"port": listener.Addr().(*net.TCPAddr).Port, "token": server.LocalToken()}
	if err := json.NewEncoder(os.Stdout).Encode(ready); err != nil {
		log.Fatal(err)
	}
	httpServer := &http.Server{Handler: server.LocalHandler()}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = httpServer.Shutdown(context.Background())
		server.Close()
	}()
	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
