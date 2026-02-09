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
	"github.com/tusk-framework/tusk-engine/internal/php"
)

// Process represents a single PHP worker process
type Process struct {
	cmd       *exec.Cmd
	ID        int
	CreatedAt time.Time
	Stdin     io.WriteCloser
	Stdout    io.ReadCloser
	Enc       *json.Encoder
	Dec       *json.Decoder
}

// Pool manages a set of PHP worker processes
type Pool struct {
	cfg     *config.Config
	phpMgr  *php.Manager
	workers []*Process
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewPool creates a new worker pool
func NewPool(cfg *config.Config) (*Pool, error) {
	// Initialize PHP Manager
	mgr, err := php.NewManager(cfg.PhpBinary)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PHP manager: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{
		cfg:    cfg,
		phpMgr: mgr,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Start spawns the configured number of workers
func (p *Pool) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Printf("Starting %d PHP workers...", p.cfg.WorkerCount)

	for i := 0; i < p.cfg.WorkerCount; i++ {
		if err := p.spawnWorker(i); err != nil {
			return err
		}
	}
	return nil
}

// spawnWorker starts a single PHP process
func (p *Pool) spawnWorker(id int) error {
	// Worker script path
	workerScript := p.cfg.WorkerCommand
	if !filepath.IsAbs(workerScript) {
		workerScript = filepath.Join(p.cfg.ProjectRoot, workerScript)
	}

	// Construct arguments
	args := []string{workerScript}

	// If a custom php.ini is provided
	if p.cfg.PhpIni != "" {
		args = append([]string{"-c", p.cfg.PhpIni}, args...)
	}

	cmd := exec.Command(p.phpMgr.BinaryPath, args...)

	// Wire up Pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// stderr can go to main log
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
	}
	p.workers = append(p.workers, worker)

	// Watch the process in a goroutine
	go p.watchWorker(worker)

	return nil
}

// watchWorker monitors a worker process and restarts it if it exits
func (p *Pool) watchWorker(worker *Process) {
	err := worker.cmd.Wait()

	// Check if the pool is shutting down
	select {
	case <-p.ctx.Done():
		return
	default:
	}

	log.Printf("Worker %d exited: %v. Restarting...", worker.ID, err)

	// Simple backoff
	time.Sleep(1 * time.Second)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.spawnWorker(worker.ID)
}

// HandleRequest dispatches a request to an available worker
func (p *Pool) HandleRequest(req map[string]interface{}) (map[string]interface{}, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.workers) == 0 {
		return nil, fmt.Errorf("no workers available")
	}

	// Pickup first worker (Round-robin logic can be added later)
	w := p.workers[0]

	// Send
	if err := w.Enc.Encode(req); err != nil {
		return nil, err
	}

	// Receive
	var resp map[string]interface{}
	if err := w.Dec.Decode(&resp); err != nil {
		return nil, err
	}

	return resp, nil
}

// Stop terminates all workers
func (p *Pool) Stop() {
	p.cancel()
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, w := range p.workers {
		if w.cmd.Process != nil {
			w.cmd.Process.Kill()
		}
	}
}
