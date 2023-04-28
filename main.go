package main

import (
	"log"
	"model-hub/api"
	"model-hub/config"
	"model-hub/helper"
	"model-hub/workers"
)

func main() {
	cfg, err := config.Load(helper.GetEnv("CONFIG_PATH", "config.yaml"))
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	workerManager := workers.NewWorkerManager(cfg)
	log.Println("Starting workers")
	if err := workerManager.Initialize(); err != nil {
		log.Fatalf("Failed to initialize worker manager: %v", err)
	}

	api.NewAPIServer(workerManager)
}
