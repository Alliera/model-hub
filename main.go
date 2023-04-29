package main

import (
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"log"
	"model-hub/api"
	"model-hub/config"
	"model-hub/helper"
	"model-hub/workers"
)

func newTextLogger() (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.TimeKey = ""
	cfg.Encoding = "console"
	return cfg.Build()
}

func main() {
	cfg, err := config.Load(helper.GetEnv("CONFIG_PATH", "config.yaml"))
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger, err := newTextLogger()
	if err != nil {
		panic(fmt.Sprintf("failed to create logger: %v", err))
	}
	defer func() { _ = logger.Sync() }()

	workerManager := workers.NewWorkerManager(cfg, logger)
	logger.Info("Starting workers")
	workerManager.Initialize()

	api.NewAPIServer(workerManager, logger)
}
