package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"worker/app"
	"worker/client"
	"worker/config"
	"worker/database"
	"worker/downloader"
	"worker/transcoder"
	"worker/webrtc"
)

var (
	gatewayURL = flag.String("gateway", "ws://localhost:8080/ws/nodes", "Gateway WebSocket URL")
	nodeID     = flag.String("id", "", "Worker node ID (auto-generated if empty)")
	nodeName   = flag.String("name", "", "Worker node name")
	configFile = flag.String("config", "config/worker.json", "Configuration file path")
)

func main() {
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Printf("Failed to load config, using defaults: %v", err)
		cfg = config.Default()
	}

	if *gatewayURL != "ws://localhost:8080/ws/nodes" {
		cfg.Gateway.URL = *gatewayURL
	}
	if *nodeID != "" {
		cfg.Node.ID = *nodeID
	}
	if *nodeName != "" {
		cfg.Node.Name = *nodeName
	}

	if err := cfg.GetStoragePaths(); err != nil {
		log.Fatalf("Failed to create storage paths: %v", err)
	}

	if err := database.Initialize("data/config"); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	deps := app.Dependencies{
		Gateway:    client.New(cfg.Gateway.URL, cfg.Node.ID),
		Downloader: downloader.New(cfg.Storage.DownloadPath, cfg.Node.ID),
		Transcoder: transcoder.New(cfg.Storage.DownloadPath, cfg.Storage.M3U8Path),
		WebRTC:     webrtc.New(),
	}

	worker, err := app.New(cfg, deps)
	if err != nil {
		log.Fatalf("Failed to create worker: %v", err)
	}

	log.Printf("Worker Node starting: ID=%s, Name=%s", cfg.Node.ID, cfg.Node.Name)
	log.Printf("Gateway URL: %s", cfg.Gateway.URL)

	if err := worker.Start(); err != nil {
		log.Fatalf("Failed to start worker node: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down worker node...")
	worker.Stop()
}
