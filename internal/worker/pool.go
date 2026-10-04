package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/config"
	"github.com/tusk-framework/tusk-engine/internal/control"
	"github.com/tusk-framework/tusk-engine/internal/metrics"
	"github.com/tusk-framework/tusk-engine/internal/php"
)

type commandFactory func(string, ...string) *exec.Cmd

// Process represents a single PHP worker process.
type Process struct {
	cmd       *exec.Cmd
	ID        int
	CreatedAt time.Time
	Stdin     io.WriteCloser
	Stdout    io.ReadCloser
	Enc       *json.Encoder
	Dec       *json.Decoder

	mu    sync.Mutex
	alive bool
}

func (p *Process) isAlive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.alive
}

func (p *Process) markDead() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	wasAlive := p.alive
	p.alive = false
	return wasAlive
}

// Pool manages a set of PHP worker processes.
type Pool struct {
	cfg         *config.Config
	phpMgr      *php.Manager
	workers     []*Process
	workerQueue chan *Process
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	newCommand  commandFactory
	snapshot    control.RuntimeSnapshot
}

// NewPool creates a new worker pool.
func NewPool(cfg *config.Config) (*Pool, error) {
	return newPoolWithCommandFactory(cfg, exec.Command)
}

func newPoolWithCommandFactory(cfg *config.Config, factory commandFactory) (*Pool, error) {
	if cfg.WorkerCount <= 0 {
		return nil, fmt.Errorf("worker_count must be positive")
	}
	mgr, err := php.NewManager(cfg.PhpBinary)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PHP manager: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now().UTC()
	return &Pool{
		cfg:         cfg,
		phpMgr:      mgr,
		workerQueue: make(chan *Process, cfg.WorkerCount),
		ctx:         ctx,
		cancel:      cancel,
		newCommand:  factory,
		snapshot: control.RuntimeSnapshot{
			EngineState:       control.EngineStarting,
			ReadinessReason:   control.ReadinessStarting,
			DesiredWorkers:    cfg.WorkerCount,
			WorkerCountsKnown: true,
			StartedAt:         now,
			StateChangedAt:    now,
		},
	}, nil
}

// Snapshot returns a point-in-time copy of Engine and worker-pool state.
func (p *Pool) Snapshot() control.RuntimeSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.snapshot
}

func (p *Pool) setEngineStateLocked(state control.EngineState) {
	if p.snapshot.EngineState != state {
		p.snapshot.EngineState = state
		p.snapshot.StateChangedAt = time.Now().UTC()
	}

	p.refreshWorkerSnapshotLocked()
}

func (p *Pool) recordWorkerFailureLocked(category string) {
	if category == "worker_crashed" {
		p.snapshot.WorkerCrashes++
	}
	if category == "restart_failed" {
		p.snapshot.WorkerRestarts++
	}
	p.snapshot.LastErrorCategory = category
}

func (p *Pool) refreshWorkerSnapshotLocked() {
	p.snapshot.TotalWorkers = len(p.workers)
	p.snapshot.ReadyWorkers = len(p.workerQueue)
	p.snapshot.ActiveWorkers = p.snapshot.TotalWorkers - p.snapshot.ReadyWorkers
	if p.snapshot.ActiveWorkers < 0 {
		p.snapshot.ActiveWorkers = 0
	}

	switch p.snapshot.EngineState {
	case control.EngineStopping:
		p.snapshot.ReadinessReason = control.ReadinessStopping
	case control.EngineStopped:
		p.snapshot.ReadinessReason = control.ReadinessStopping
	case control.EngineStarting:
		p.snapshot.ReadinessReason = control.ReadinessStarting
	case control.EngineRunning:
		if p.snapshot.ReadyWorkers > 0 {
			p.snapshot.ReadinessReason = control.ReadinessReady
		} else if p.snapshot.LastErrorCategory == "worker_crashed" || p.snapshot.LastErrorCategory == "restart_failed" {
			p.snapshot.ReadinessReason = control.ReadinessWorkerCrashed
		} else {
			p.snapshot.ReadinessReason = control.ReadinessNoWorkers
		}
	}
}

// Start spawns the configured number of workers.
func (p *Pool) Start() error {
	log.Printf("Starting %d PHP workers...", p.cfg.WorkerCount)
	metrics.WorkersTotal.Set(float64(p.cfg.WorkerCount))

	for i := 0; i < p.cfg.WorkerCount; i++ {
		if err := p.spawnWorker(i); err != nil {
			p.mu.Lock()
			p.recordWorkerFailureLocked("startup_failed")
			p.refreshWorkerSnapshotLocked()
			p.mu.Unlock()
			p.Stop()
			return err
		}
	}

	p.mu.Lock()
	p.setEngineStateLocked(control.EngineRunning)
	p.mu.Unlock()
	return nil
}

func (p *Pool) spawnWorker(id int) error {
	workerScript := p.cfg.WorkerCommand
	if !filepath.IsAbs(workerScript) {
		workerScript = filepath.Join(p.cfg.ProjectRoot, workerScript)
	}
	if _, err := os.Stat(workerScript); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worker script not found: %s", workerScript)
		}
		return fmt.Errorf("stat worker script %s: %w", workerScript, err)
	}

	args := []string{workerScript}
	if p.cfg.PhpIni != "" {
		args = append([]string{"-c", p.cfg.PhpIni}, args...)
	}
	cmd := p.newCommand(p.phpMgr.BinaryPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start worker %d: %w", id, err)
	}

	worker := &Process{
		cmd:       cmd,
		ID:        id,
		CreatedAt: time.Now(),
		Stdin:     stdin,
		Stdout:    stdout,
		Enc:       json.NewEncoder(stdin),
		Dec:       json.NewDecoder(stdout),
		alive:     true,
	}

	p.mu.Lock()
	p.workers = append(p.workers, worker)
	p.mu.Unlock()

	select {
	case p.workerQueue <- worker:
	case <-p.ctx.Done():
		p.terminate(worker)
		p.removeWorker(worker)
		return fmt.Errorf("pool shutting down")
	}

	p.mu.Lock()
	p.refreshWorkerSnapshotLocked()
	p.mu.Unlock()

	go p.watchWorker(worker)
	return nil
}

func (p *Pool) watchWorker(worker *Process) {
	err := worker.cmd.Wait()
	worker.markDead()
	p.removeWorker(worker)

	select {
	case <-p.ctx.Done():
		return
	default:
	}

	log.Printf("Worker %d exited: %v. Restarting...", worker.ID, err)
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-p.ctx.Done():
		return
	case <-timer.C:
	}

	select {
	case <-p.ctx.Done():
		return
	default:
	}
	p.mu.Lock()
	p.recordWorkerFailureLocked("worker_crashed")
	p.refreshWorkerSnapshotLocked()
	p.mu.Unlock()
	if err := p.spawnWorker(worker.ID); err != nil {
		log.Printf("Worker %d restart failed: %v", worker.ID, err)
		p.mu.Lock()
		p.recordWorkerFailureLocked("restart_failed")
		p.refreshWorkerSnapshotLocked()
		p.mu.Unlock()
		return
	}
	p.mu.Lock()
	p.snapshot.WorkerRestarts++
	p.refreshWorkerSnapshotLocked()
	p.mu.Unlock()
}

func (p *Pool) removeWorker(target *Process) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, worker := range p.workers {
		if worker == target {
			p.workers = append(p.workers[:i], p.workers[i+1:]...)
			return
		}
	}
}

func (p *Pool) terminate(worker *Process) {
	if !worker.markDead() {
		return
	}
	if worker.cmd.Process != nil {
		_ = worker.cmd.Process.Kill()
	}
}

// HandleRequest dispatches a request to an available live worker.
func (p *Pool) HandleRequest(req map[string]interface{}) (map[string]interface{}, error) {
	var worker *Process
	for {
		select {
		case candidate := <-p.workerQueue:
			if candidate.isAlive() {
				worker = candidate
				goto leased
			}
		case <-p.ctx.Done():
			return nil, fmt.Errorf("pool shutting down")
		}
	}
leased:
	p.mu.Lock()
	p.refreshWorkerSnapshotLocked()
	p.mu.Unlock()
	p.wg.Add(1)
	defer p.wg.Done()
	defer p.release(worker)

	errCh := make(chan error, 1)
	respCh := make(chan map[string]interface{}, 1)
	go func() {
		if err := worker.Enc.Encode(req); err != nil {
			errCh <- fmt.Errorf("worker %d encode error: %w", worker.ID, err)
			return
		}
		var resp map[string]interface{}
		if err := worker.Dec.Decode(&resp); err != nil {
			errCh <- fmt.Errorf("worker %d decode error: %w", worker.ID, err)
			return
		}
		respCh <- resp
	}()

	timeoutDuration := time.Duration(p.cfg.Timeout) * time.Second
	if timeoutDuration <= 0 {
		timeoutDuration = 30 * time.Second
	}
	timer := time.NewTimer(timeoutDuration)
	defer timer.Stop()
	select {
	case resp := <-respCh:
		return resp, nil
	case err := <-errCh:
		p.terminate(worker)
		return nil, err
	case <-timer.C:
		p.terminate(worker)
		return nil, fmt.Errorf("worker %d timed out after %s", worker.ID, timeoutDuration)
	}
}

func (p *Pool) release(worker *Process) {
	if !worker.isAlive() {
		p.mu.Lock()
		p.refreshWorkerSnapshotLocked()
		p.mu.Unlock()
		return
	}
	select {
	case p.workerQueue <- worker:
	case <-p.ctx.Done():
		p.terminate(worker)
	}
	p.mu.Lock()
	p.refreshWorkerSnapshotLocked()
	p.mu.Unlock()
}

// Stop terminates all workers and prevents future restarts.
func (p *Pool) Stop() {
	p.mu.Lock()
	if p.snapshot.EngineState == control.EngineStopped {
		p.mu.Unlock()
		return
	}
	p.setEngineStateLocked(control.EngineStopping)
	p.mu.Unlock()

	p.cancel()

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Println("Timeout waiting for requests, killing workers...")
	}

	p.mu.Lock()
	workers := p.workers
	p.workers = nil
	p.mu.Unlock()
	for _, worker := range workers {
		p.terminate(worker)
	}
	p.mu.Lock()
	p.setEngineStateLocked(control.EngineStopped)
	p.mu.Unlock()
}
