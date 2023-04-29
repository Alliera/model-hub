package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/shirou/gopsutil/v3/process"
	"go.uber.org/zap"
	"io"
	"model-hub/config"
	"model-hub/models"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

type Worker struct {
	ID               WorkerId
	Model            config.Model
	IsLoaded         bool
	IsBusy           bool
	startTime        time.Time
	cmd              *exec.Cmd
	port             int
	mu               sync.Mutex
	failedWorkerChan chan WorkerId
	ctx              context.Context
	cancel           context.CancelFunc
	logger           *zap.Logger
}

func NewWorker(id WorkerId, model config.Model, port int, failedWorkerChan chan WorkerId, logger *zap.Logger) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	return &Worker{
		ID:               id,
		Model:            model,
		IsLoaded:         false,
		IsBusy:           false,
		port:             port,
		failedWorkerChan: failedWorkerChan,
		ctx:              ctx,
		cancel:           cancel,
		logger:           logger,
	}
}

func (w *Worker) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	cmd := exec.Command("python3", "worker.py", string(w.ID), w.Model.Path, strconv.Itoa(w.port), w.Model.Handler)

	cmd.Stdout = zap.NewStdLog(w.logger).Writer()
	cmd.Stderr = zap.NewStdLog(w.logger).Writer()

	if err := cmd.Start(); err != nil {
		// In case than worker can't start, no use completing
		panic(fmt.Sprintf("failed to start worker %s: %v", w.ID, err))
	}
	w.startTime = time.Now()
	go logResourceUsage(w.ctx, cmd.Process.Pid, w.ID, w.logger)

	go func() {
		err := cmd.Wait()
		if err != nil {
			elapsedTime := time.Since(w.startTime)
			hours := int(elapsedTime.Hours())
			minutes := int(elapsedTime.Minutes()) % 60
			seconds := int(elapsedTime.Seconds()) % 60

			// Форматируем строку с временем
			timeString := ""
			if hours > 0 {
				timeString = fmt.Sprintf("%d hours ", hours)
			}
			if minutes > 0 {
				timeString += fmt.Sprintf("%d minutes ", minutes)
			}
			timeString += fmt.Sprintf("%d seconds", seconds)

			w.logger.Error(fmt.Sprintf("Worker %s: command exited with error: %v, worked for %s", w.ID, err, timeString))
			w.failedWorkerChan <- w.ID
		}
	}()

	w.cmd = cmd
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

func logResourceUsage(ctx context.Context, pid int, workerId WorkerId, logger *zap.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			p, err := process.NewProcess(int32(pid))
			if err != nil {
				logger.Error("Failed to get process", zap.String("workerId", string(workerId)), zap.Error(err))
				continue
			}
			cpuPercent, err := p.CPUPercent()
			if err != nil {
				logger.Error("Failed to get CPU usage", zap.String("workerId", string(workerId)), zap.Error(err))
			}

			memInfo, err := p.MemoryInfo()
			if err != nil {
				logger.Error("Failed to get memory usage", zap.String("workerId", string(workerId)), zap.Error(err))
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
			logger.Info(fmt.Sprintf("⚙️ Worker [%s]: 🖥️ CPU: %.2f%% | 💾 RAM: %.2f MB | 🎮 GPU: %.2f%%",
				workerId, cpuPercent, ramInMB, gpuPercent))
		}

		time.Sleep(30 * time.Second)
	}
}
