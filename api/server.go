package api

import (
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"model-hub/helper"
	"model-hub/workers"
	"time"
)

func NewAPIServer(manager *workers.WorkerManager, logger *zap.Logger) {
	handlers := NewHandlers(manager, logger)

	r := gin.New()
	r.Use(ginzap.Ginzap(logger, time.RFC3339, true))
	r.Use(ginzap.RecoveryWithZap(logger, true))

	r.POST("/predict", handlers.PredictHandler)
	r.GET("/ping", handlers.PingHandler)
	r.POST("/model-ready", handlers.ModelReady)

	addr := "0.0.0.0:" + helper.GetEnv("SERVER_PORT", "7766")
	logger.Info("Starting server...")
	if err := r.Run(addr); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}
