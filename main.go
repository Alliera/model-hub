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
	workerManager.Initialize()

	api.NewAPIServer(workerManager)
}
