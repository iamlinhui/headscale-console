package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"meshconsole/internal/auth"
	"meshconsole/internal/config"
	"meshconsole/internal/webassets"
	"meshconsole/internal/webserver"
)

func main() {
	var cfg config.Config
	var err error
	if len(os.Args) == 2 && os.Args[1] == "preview" {
		pw := make([]byte, 12)
		if _, err = rand.Read(pw); err != nil {
			log.Fatal("cannot generate preview credentials")
		}
		password := base64.RawURLEncoding.EncodeToString(pw)
		hash, e := auth.HashPassword(password)
		if e != nil {
			log.Fatal("cannot hash preview password")
		}
		values := map[string]string{"APP_ENV": "development", "HOST": "127.0.0.1", "PORT": "3080", "CONSOLE_ORIGIN": "http://127.0.0.1:3080", "COOKIE_SECURE": "false", "DEMO_MODE": "true", "CONSOLE_USERNAME": "admin", "CONSOLE_PASSWORD_HASH": hash, "HEADSCALE_URL": "http://headscale:9981", "HEADSCALE_PUBLIC_URL": "https://headscale.example.com", "HEADSCALE_API_KEY": "DEMO_INVALID_SERVER_ONLY"}
		cfg, err = config.Load(func(k string) string { return values[k] })
		fmt.Printf("Local Go preview: http://127.0.0.1:3080\nTemporary username: admin\nTemporary password: %s\n", password)
	} else {
		flags := flag.NewFlagSet("mesh-console", flag.ExitOnError)
		configPath := flags.String("config", "config.yaml", "path to console YAML configuration")
		_ = flags.Parse(os.Args[1:])
		if flags.NArg() != 0 {
			log.Fatal("usage: mesh-console [-config config.yaml] or mesh-console preview")
		}
		cfg, err = config.LoadFile(*configPath, os.LookupEnv)
	}
	if err != nil {
		log.Fatalf("startup refused: %v", err)
	}
	frontend, err := fs.Sub(webassets.Frontend, "static/dist")
	if err != nil {
		log.Fatal("embedded frontend unavailable")
	}
	handler, err := webserver.New(cfg, frontend)
	if err != nil {
		log.Fatal("invalid server or upstream configuration")
	}
	server := &http.Server{Addr: net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 70 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 16}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	shutdownDone := make(chan struct{})
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		close(shutdownDone)
	}()
	log.Printf("Go console listening on %s", server.Addr)
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("listener failed; check bind address and port")
	}
	<-shutdownDone
}
