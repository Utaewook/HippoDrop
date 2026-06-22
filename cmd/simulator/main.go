package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"tardis/internal/simulator"
)

func main() {
	configPath := flag.String("config", "simulator.yml", "Path to simulator config file")
	flag.Parse()

	cfg, err := simulator.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	pollDur, err := time.ParseDuration(cfg.PollInterval)
	if err != nil {
		log.Fatalf("❌ Invalid poll_interval: %v", err)
	}

	fmt.Printf("🚀 Starting Tardis Simulator targeting %s\n", cfg.TargetURL)
	fmt.Printf("📂 Workspace: %s\n", cfg.Workspace)

	tracker := simulator.NewTracker(cfg.TargetURL, pollDur)
	go tracker.Start()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	for _, svc := range cfg.Services {
		wg.Add(1)
		go func(s simulator.ServiceConfig) {
			defer wg.Done()
			runService(ctx, s, cfg.Workspace, tracker)
		}(svc)
	}

	<-ctx.Done()
	fmt.Println("\n🛑 Simulator shutting down...")
	wg.Wait()
	fmt.Println("✅ Simulation ended cleanly.")
}

func runService(ctx context.Context, svc simulator.ServiceConfig, workspace string, tracker *simulator.Tracker) {
	interval, err := time.ParseDuration(svc.Interval)
	if err != nil {
		log.Printf("❌ [%s] Invalid interval %s: %v", svc.Name, svc.Interval, err)
		return
	}

	log.Printf("🔥 [%s] Started | Interval: %s | Size: %dKB", svc.Name, svc.Interval, svc.SizeKB)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			localPath, err := simulator.GenerateFakeFile(workspace, svc.Name, svc.SizeKB)
			if err != nil {
				log.Printf("⚠️ [%s] Failed to generate file: %v", svc.Name, err)
				continue
			}

			// Derive remote path
			fileName := filepath.Base(localPath)
			remotePath := filepath.Join(svc.RemoteDir, fileName)

			if err := tracker.UploadFile(localPath, remotePath); err != nil {
				log.Printf("⚠️ [%s] Failed to upload task: %v", svc.Name, err)
			} else {
				log.Printf("📤 [%s] Enqueued %s", svc.Name, fileName)
			}
		}
	}
}
