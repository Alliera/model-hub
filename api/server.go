package api

import (
	"fmt"
	"github.com/gorilla/mux"
	"model-hub/helper"
	"model-hub/workers"
	"net/http"
	"time"
)

func NewAPIServer(manager *workers.WorkerManager) {
	handlers := NewHandlers(manager)
	r := mux.NewRouter()
	r.HandleFunc("/predict", handlers.PredictHandler).Methods(http.MethodPost)
	r.HandleFunc("/ping", handlers.PingHandler).Methods(http.MethodGet)
	r.HandleFunc("/model-ready", handlers.ModelReady).Methods(http.MethodPost)

	srv := &http.Server{
		Addr:         "0.0.0.0:" + helper.GetEnv("SERVER_PORT", "7766"),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	fmt.Println("Starting server...")
	if err := srv.ListenAndServe(); err != nil {
		panic(fmt.Sprintf("failed to start server: %v", err))
	}
}
