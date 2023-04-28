package workers

import (
	"fmt"
	"model-hub/config"
	"model-hub/models"
)

type WorkerId string

type WorkerManager struct {
	workers    map[WorkerId]*Worker
	workerChan map[models.ModelName]chan *Worker
	modelNames []models.ModelName
}

func NewWorkerManager(cfg *config.Config) *WorkerManager {
	workers := make(map[WorkerId]*Worker)
	workerChan := make(map[models.ModelName]chan *Worker)
	var modelNames []models.ModelName
	port := 7777
	for _, model := range cfg.Models {
		modelNames = append(modelNames, model.Name)
		workerChan[model.Name] = make(chan *Worker, model.Workers)
		for i := 1; i <= model.Workers; i++ {
			port += 1
			workerID := WorkerId(fmt.Sprintf("%s-%d", model.Name, i))
			worker := NewWorker(workerID, model, port)
			workers[workerID] = worker
		}
	}

	return &WorkerManager{
		workers:    workers,
		workerChan: workerChan,
		modelNames: modelNames,
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

func (wm *WorkerManager) Initialize() error {
	for _, worker := range wm.workers {
		if err := worker.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (wm *WorkerManager) GetAvailableWorker(modelName models.ModelName) (*Worker, error) {
	workerChan, ok := wm.workerChan[modelName]
	if !ok {
		return nil, fmt.Errorf("no worker channel for the requested model: %s", modelName)
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
