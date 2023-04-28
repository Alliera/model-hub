package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"model-hub/models"
	"model-hub/workers"
	"net/http"
)

type Handlers struct {
	manager *workers.WorkerManager
}

func NewHandlers(manager *workers.WorkerManager) *Handlers {
	return &Handlers{manager: manager}
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func writeErrorJSON(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
	if err != nil {
		log.Println("failed to encode error JSON:", err)
	}
}

func (h *Handlers) PredictHandler(w http.ResponseWriter, r *http.Request) {
	var req models.PredictRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "failed to decode request body")
		return
	}
	modelString, ok := req.Params["model"].(string)
	if !ok {
		writeErrorJSON(w, http.StatusBadRequest, "model parameter is missing or has an invalid format")
		return
	}
	model := models.ModelName(modelString)

	worker, err := h.manager.GetAvailableWorker(model)
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "failed to get available worker")
		return
	}

	preds, err := worker.Predict(req)
	h.manager.SetWorkerAvailable(worker.ID)
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "failed to get predictions: "+err.Error())
		return
	}

	err = json.NewEncoder(w).Encode(&preds)
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "failed to encode response body")
		return
	}
}

func (h *Handlers) PingHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	//if h.manager.IsReady() {
	//	w.WriteHeader(http.StatusOK)
	//	return
	//}
	//
	//writeErrorJSON(w, http.StatusServiceUnavailable, "models are not ready")
}

func (h *Handlers) ModelReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var data struct {
		WorkerId workers.WorkerId `json:"worker_id"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		fmt.Println("failed to unmarshal request body!")
		writeErrorJSON(w, http.StatusBadRequest, "failed to unmarshal request body")
		return
	}

	h.manager.SetWorkerAvailable(data.WorkerId)

	w.WriteHeader(http.StatusOK)
}
