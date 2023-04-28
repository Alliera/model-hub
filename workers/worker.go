package workers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/shirou/gopsutil/v3/process"
	"io"
	"model-hub/config"
	"model-hub/models"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
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

	go logResourceUsage(cmd.Process.Pid, w.ID) // вызов функции логирования в отдельной горутине

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

func logResourceUsage(pid int, workerId WorkerId) {
	for {
		time.Sleep(30 * time.Second)
		p, err := process.NewProcess(int32(pid))
		if err != nil {
			fmt.Printf("Worker %s: failed to get process: %v\n", workerId, err)
			continue
		}

		cpuPercent, err := p.CPUPercent()
		if err != nil {
			fmt.Printf("Worker %s: failed to get CPU usage: %v\n", workerId, err)
		}

		memInfo, err := p.MemoryInfo()
		if err != nil {
			fmt.Printf("Worker %s: failed to get memory usage: %v\n", workerId, err)
		}

		cmd := exec.Command(
			"nvidia-smi",
			"--query-gpu=memory.used,memory.total",
			"--format=csv,noheader,nounits",
		)
		output, err := cmd.Output()
		var gpuPercent float64 = 0

		if err == nil {
			var gpuMemoryUsed, gpuMemoryTotal uint64
			_, _ = fmt.Sscanf(string(output), "%d, %d", &gpuMemoryUsed, &gpuMemoryTotal)
			gpuPercent = (float64(gpuMemoryUsed) / float64(gpuMemoryTotal)) * 100
		}

		ramInMB := float64(memInfo.RSS) / (1024 * 1024)
		fmt.Printf("⚙️ Worker [%s]: 🖥️ CPU: %.2f%% | 💾 RAM: %.2f MB | 🎮 GPU: %.2f%%\n",
			workerId, cpuPercent, ramInMB, gpuPercent)
	}
}
