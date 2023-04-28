package workers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"model-hub/config"
	"model-hub/models"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

type Worker struct {
	ID       WorkerId
	Model    config.Model
	IsLoaded bool
	IsBusy   bool
	cmd      *exec.Cmd
	port     int
	mu       sync.Mutex
}

func NewWorker(id WorkerId, model config.Model, port int) *Worker {
	return &Worker{
		ID:       id,
		Model:    model,
		IsLoaded: false,
		IsBusy:   false,
		port:     port,
	}
}

func (w *Worker) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	cmd := exec.Command("python3", "worker.py", string(w.ID), w.Model.Path, strconv.Itoa(w.port), w.Model.Handler)

	// set stdout and stderr to os.Stdout
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start worker %s: %v", w.ID, err)
	}

	go func() {
		_ = cmd.Wait()
	}()

	w.cmd = cmd

	return nil
}

func (w *Worker) SetLoaded() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.IsLoaded = true
}

func (w *Worker) SetBusy() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.IsBusy = true
}

func (w *Worker) SetAvailable() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.IsBusy = false
}

func (w *Worker) Predict(request models.PredictRequest) (response interface{}, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Marshal the request object
	reqBody, err := json.Marshal(request)
	if err != nil {
		return response, fmt.Errorf("worker %s: failed to marshal request: %v", w.ID, err)
	}

	// Create the POST request
	url := fmt.Sprintf("http://127.0.0.1:%d/predict", w.port)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return response, fmt.Errorf("worker %s: failed to create POST request: %v", w.ID, err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return response, fmt.Errorf("worker %s: failed to send POST request: %v", w.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return response, fmt.Errorf("worker %s: server returned non-200 status code: %d", w.ID, resp.StatusCode)
	}

	// Read and unmarshal the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return response, fmt.Errorf("worker %s: failed to read response body: %v", w.ID, err)
	}

	err = json.Unmarshal(respBody, &response)
	if err != nil {
		return response, fmt.Errorf("worker %s: failed to unmarshal response: %v", w.ID, err)
	}

	return response, nil
}
