package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/automationpi/pv-safe/internal/webhook"
)

var (
	port     = flag.String("port", "8443", "Port to listen on")
	certFile = flag.String("cert-file", "/etc/webhook/certs/tls.crt", "Path to TLS certificate")
	keyFile  = flag.String("key-file", "/etc/webhook/certs/tls.key", "Path to TLS key")
)

func main() {
	flag.Parse()

	logger := log.New(os.Stdout, "[pv-safe-webhook] ", log.LstdFlags|log.Lshortfile)

	logger.Println("Starting pv-safe webhook server...")
	logger.Printf("Listening on port: %s", *port)
	logger.Printf("TLS cert: %s", *certFile)
	logger.Printf("TLS key: %s", *keyFile)

	logger.Println("Initializing Kubernetes client...")
	client, config, err := webhook.NewKubernetesClient()
	if err != nil {
		logger.Fatalf("Failed to create Kubernetes client: %v", err)
	}
	logger.Println("Kubernetes client initialized successfully")

	logger.Println("Initializing Snapshot checker...")
	snapshotChecker, err := webhook.NewSnapshotChecker(config, client)
	if err != nil {
		logger.Printf("Warning: Failed to create Snapshot checker: %v", err)
		logger.Println("Snapshot support will be disabled")
		snapshotChecker = nil
	} else {
		logger.Println("Snapshot checker initialized successfully")
	}

	handler := webhook.NewHandler(logger, client, snapshotChecker)

	mux := http.NewServeMux()
	mux.Handle("/validate", handler)
	mux.HandleFunc("/healthz", handler.HealthCheck)
	mux.HandleFunc("/readyz", handler.HealthCheck)

	server := &http.Server{
		Addr:              ":" + *port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	logger.Printf("Webhook server listening on https://0.0.0.0:%s", *port)
	logger.Println("Endpoints:")
	logger.Println("  - POST /validate (admission webhook)")
	logger.Println("  - GET  /healthz  (health check)")
	logger.Println("  - GET  /readyz   (readiness check)")

	if err := server.ListenAndServeTLS(*certFile, *keyFile); err != nil {
		logger.Fatalf("Failed to start server: %v", err)
	}
}
