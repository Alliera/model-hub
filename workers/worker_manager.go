package workers

import (
	"fmt"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
	"go.uber.org/zap"
	"model-hub/config"
	"model-hub/models"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type WorkerId string

type WorkerManager struct {
	workers          map[WorkerId]*Worker
	workerChan       map[models.ModelName]chan *Worker
	failedWorkerChan chan WorkerId
	modelNames       []models.ModelName
	mu               sync.Mutex
	logger           *zap.Logger
}

type WorkerInfo struct {
	ID                WorkerId
	ElapsedTimeString string
	CPUPercent        string
	RAMInMB           string
}

func (wm *WorkerManager) logResourceUsage() {
	for {
		startTime := time.Now()
		cmd := exec.Command(
			"nvidia-smi",
			"--query-gpu=memory.used,memory.total",
			"--format=csv,noheader,nounits",
		)
		output, err := cmd.Output()

		gpuPercent := 0.0
		if err == nil {
			var gpuMemoryUsed, gpuMemoryTotal uint64
			_, _ = fmt.Sscanf(string(output), "%d, %d", &gpuMemoryUsed, &gpuMemoryTotal)
			gpuPercent = (float64(gpuMemoryUsed) / float64(gpuMemoryTotal)) * 100
		}

		// Получаем информацию о памяти
		v, _ := mem.VirtualMemory()
		totalRAM := float64(v.Total) / (1024 * 1024)
		availableRAM := float64(v.Available) / (1024 * 1024)

		// Получаем информацию о процессоре
		percentages, _ := cpu.Percent(time.Duration(1000)*time.Millisecond, true)

		var cpuInfoBuilder strings.Builder
		for i, percentage := range percentages {
			cpuInfoBuilder.WriteString(fmt.Sprintf("Core%d: %.2f%%|", i, percentage))
		}

		cpuInfo := strings.TrimSuffix(cpuInfoBuilder.String(), " | ")

		var workerInfos []WorkerInfo
		var maxIDLen, maxElapsedLen, maxCPULen, maxRAMLen int

		for _, worker := range wm.workers {
			if !worker.IsLaunched() {
				continue
			}
			p, err := process.NewProcess(int32(worker.cmd.Process.Pid))
			if err != nil {
				wm.logger.Error("Failed to get process", zap.String("workerId", string(worker.ID)), zap.Error(err))
				continue
			}
			cpuPercent, err := p.CPUPercent()
			if err != nil {
				wm.logger.Error("Failed to get CPU usage", zap.String("workerId", string(worker.ID)), zap.Error(err))
			}

			memInfo, err := p.MemoryInfo()
			if err != nil {
				wm.logger.Error("Failed to get memory usage", zap.String("workerId", string(worker.ID)), zap.Error(err))
			}

			ramInMB := float64(memInfo.RSS) / (1024 * 1024)

			idLen := len(worker.ID)
			elapsedLen := len(worker.ElapsedTimeString())
			cpuLen := len(fmt.Sprintf("%.2f", cpuPercent))
			ramLen := len(fmt.Sprintf("%.2f", ramInMB))

			if idLen > maxIDLen {
				maxIDLen = idLen
			}
			if elapsedLen > maxElapsedLen {
				maxElapsedLen = elapsedLen
			}
			if cpuLen > maxCPULen {
				maxCPULen = cpuLen
			}
			if ramLen > maxRAMLen {
				maxRAMLen = ramLen
			}

			workerInfos = append(workerInfos, WorkerInfo{
				ID:                worker.ID,
				ElapsedTimeString: worker.ElapsedTimeString(),
				CPUPercent:        fmt.Sprintf("%.2f", cpuPercent),
				RAMInMB:           fmt.Sprintf("%.2f", ramInMB),
			})
		}
		var formattedWorkerInfo []string
		for _, info := range workerInfos {
			formattedWorkerInfo = append(formattedWorkerInfo, fmt.Sprintf("⚙️ Worker %s%-*s (⏱️lifetime: %s%-*s): 🖥️ CPU: %s%-*s%% | 💾 RAM: %s%-*s MB",
				info.ID, maxIDLen-len(info.ID), "", info.ElapsedTimeString, maxElapsedLen-len(info.ElapsedTimeString), "", info.CPUPercent, maxCPULen-len(info.CPUPercent), "", info.RAMInMB, maxRAMLen-len(info.RAMInMB), ""))

		}
		// Выводим всю информацию
		fmt.Printf("====== 🎮 TOTAL GPU USAGE:  %.2f%% =======\n", gpuPercent)
		fmt.Printf("====== 💾 TOTAL RAM: %.2f MB | AVAILABLE RAM: %.2f MB =======\n", totalRAM, availableRAM)
		fmt.Printf("====== 🖥️ CPU USAGE (%s)=======\n", cpuInfo)
		fmt.Printf("====== 🤖 WORKER INFO =======\n%s\n", strings.Join(formattedWorkerInfo, "\n"))
		fmt.Printf("====== ⏱️ TIME TAKEN FOR METRICS: %.2f s =======\n", time.Since(startTime).Seconds())
		time.Sleep(30 * time.Second)
	}
}

func (wm *WorkerManager) removeWorkerFromChannel(worker *Worker) {
	workerChan := wm.workerChan[worker.Model.Name]
	updatedChan := make(chan *Worker, cap(workerChan))

	close(workerChan)
	for w := range workerChan {
		if w.ID != worker.ID {
			updatedChan <- w
		}
	}

	wm.workerChan[worker.Model.Name] = updatedChan
}

func (wm *WorkerManager) handleFailedWorker() {
	for {
		failedWorkerID := <-wm.failedWorkerChan
		worker, ok := wm.workers[failedWorkerID]
		if ok {
			go func() {
				worker.SetUnLoaded()
				worker.SetExited()
				wm.removeWorkerFromChannel(worker)
				wm.logger.Info(fmt.Sprintf("Worker %s: Waiting 5 seconds before restarting", worker.ID))
				time.Sleep(5 * time.Second)
				worker.Start()
			}()
		}
	}
}

func NewWorkerManager(cfg *config.Config, logger *zap.Logger) *WorkerManager {
	workers := make(map[WorkerId]*Worker)
	workerChan := make(map[models.ModelName]chan *Worker)
	var modelNames []models.ModelName
	port := 7777
	failedWorkerChan := make(chan WorkerId)

	for _, model := range cfg.Models {
		modelNames = append(modelNames, model.Name)
		workerChan[model.Name] = make(chan *Worker, model.Workers)
		for i := 1; i <= model.Workers; i++ {
			port += 1
			workerID := WorkerId(fmt.Sprintf("%s-%d", model.Name, i))
			worker := NewWorker(workerID, model, port, failedWorkerChan, logger)
			workers[workerID] = worker
		}
	}
	return &WorkerManager{
		workers:          workers,
		workerChan:       workerChan,
		failedWorkerChan: failedWorkerChan,
		modelNames:       modelNames,
		logger:           logger,
	}
}

func (wm *WorkerManager) IsReady() bool {
	for _, modelName := range wm.modelNames {
		hasAnyLoaded := false
		for _, worker := range wm.workers {
			if worker.Model.Name == modelName && worker.Loaded {
				hasAnyLoaded = true
			}
		}
		if hasAnyLoaded == false {
			return false
		}
	}
	return true
}

func (wm *WorkerManager) Initialize() {
	go wm.handleFailedWorker()
	go wm.logResourceUsage()
	for _, worker := range wm.workers {
		worker.Start()
	}
}

func (wm *WorkerManager) GetAvailableWorker(modelName models.ModelName) (*Worker, error) {
	workerChan, ok := wm.workerChan[modelName]
	if !ok {
		return nil, fmt.Errorf("no worker channel for the requested model:%s", modelName)
	}

	worker := <-workerChan
	worker.SetBusy()
	return worker, nil
}

func (wm *WorkerManager) SetWorkerAvailable(workerID WorkerId) {
	worker, ok := wm.workers[workerID]
	if ok {
		worker.SetAvailable()
		wm.workerChan[worker.Model.Name] <- worker
	}
}
