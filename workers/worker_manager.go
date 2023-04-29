package workers

import (
	"fmt"
	"go.uber.org/zap"
	"model-hub/config"
	"model-hub/models"
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

func (wm *WorkerManager) updateWorkerChannel(modelName models.ModelName, newChan chan *Worker) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.workerChan[modelName] = newChan
}

func (wm *WorkerManager) removeWorkerFromChannel(worker *Worker) {
	workerChan := wm.workerChan[worker.Model.Name]
	updatedChan := make(chan *Worker, cap(workerChan))

	for {
		if len(workerChan) == 0 {
			break
		}

		w := <-workerChan
		if w.ID != worker.ID {
			updatedChan <- w
		}
	}

	wm.updateWorkerChannel(worker.Model.Name, updatedChan)
}

func (wm *WorkerManager) HandleFailedWorker() {
	for {
		failedWorkerID := <-wm.failedWorkerChan
		worker, ok := wm.workers[failedWorkerID]
		if ok {
			wm.removeWorkerFromChannel(worker)
			wm.logger.Info(fmt.Sprintf("Worker %s: Waiting 5 seconds before restarting", worker.ID))
			time.Sleep(5 * time.Second)
			worker.Start()
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
			if worker.Model.Name == modelName && worker.IsLoaded {
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
	go wm.HandleFailedWorker()
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
