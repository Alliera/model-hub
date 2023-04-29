package main

import (
	"fmt"
	"go.uber.org/zap"
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

	logger, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("failed to create logger: %v", err))
	}
	defer logger.Sync()

	workerManager := workers.NewWorkerManager(cfg, logger)
	logger.Info("Starting workers")
	workerManager.Initialize()

	api.NewAPIServer(workerManager, logger)
}
