package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"sis2/internal/backend/platform"

	log "github.com/sirupsen/logrus"
)

func main() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	blogPlatform := platform.NewBlogPlatform()
	if blogPlatform == nil {
		log.Fatalf("Failed to initialize blogPlatform")
		return
	}
	if err := blogPlatform.Start(); err != nil {
		log.Fatalf("Error starting blogPlatform. %v", err)
	}
	time.Sleep(time.Second)
	if blogPlatform.IsRunning() {
		sig := <-sigChan
		log.Infof("Received signal: %v. Initiating shutdown...", sig)
	}
	if err := blogPlatform.Stop(); err != nil {
		log.Fatalf("Error stopping blogPlatform: %v", err)
	}
	log.Infof("Shutdown complete. Exiting application.")
}
