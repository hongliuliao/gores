package gores

import (
	"errors"
	"log"
	"sync"

	"github.com/deckarep/golang-set"
)

// Dispatcher represents the dispatcher between Redis server and workers
type Dispatcher struct {
	gores       *Gores
	maxWorkers  int
	jobChannel  chan *Job
	jobChan     chan *Job
	doneChannel chan int
	queues      mapset.Set
	timeout     int
}

// NewDispatcher creates Dispatcher instance
func NewDispatcher(gores *Gores, config *Config, queues mapset.Set) *Dispatcher {
	if gores == nil || config.MaxWorkers <= 0 {
		log.Println("Invalid number of workers to initialize Dispatcher")
		return nil
	}

	return &Dispatcher{
		gores:      gores,
		maxWorkers: config.MaxWorkers,
		jobChannel: make(chan *Job, config.MaxWorkers),
		jobChan:    make(chan *Job),
		queues:     queues,
		timeout:    config.DispatcherTimeout,
	}
}

// Start starts dispatching in fanout way
func (disp *Dispatcher) Start(tasks *map[string]interface{}) error {
	var wg sync.WaitGroup
	config := disp.gores.config
	workers := make([]*Worker, disp.maxWorkers)

	for i := 0; i < disp.maxWorkers; i++ {
		worker := NewWorker(config, disp.queues, i+1)
		if worker == nil {
			return errors.New("run dispatcher failed: worker is nil")
		}
		workers[i] = worker

		wg.Add(1)
		go func() {
			defer wg.Done()
			err := worker.Start(disp, tasks)
			if err != nil {
				log.Fatalf("run dispatcher failed: %s", err)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		disp.dispatch(workers)
	}()
	wg.Wait()
	return nil
}

// dispatch dispatches jobs between Redis and Gores workers
func (disp *Dispatcher) dispatch(workers []*Worker) {
	go func() {
		for {
			job, err := ReserveJob(disp.gores, disp.queues)
			if err != nil {
				log.Printf("dispatch job failed: %s\n", err)
				return
			}

            // no job
            if job.queue != "" {
			    disp.jobChan <- job
            }
		}
	}()

	for {
		for _, worker := range workers {
			select {
			case job, ok := <-disp.jobChan:
				if !ok {
					return
				}
				worker.jobChan <- job
			}
		}
	}
}
