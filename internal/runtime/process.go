package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// ExecProcessFactory starts and supervises an external RoadRunner process.
type ExecProcessFactory struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (f ExecProcessFactory) Start(spec ProcessSpec) (Process, error) {
	if spec.Binary == "" {
		return nil, fmt.Errorf("start process: binary is required")
	}
	command := exec.Command(spec.Binary, spec.Args...)
	command.Dir = spec.Dir
	if f.Stdout != nil {
		command.Stdout = f.Stdout
	}
	if f.Stderr != nil {
		command.Stderr = f.Stderr
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start process %q: %w", spec.Binary, err)
	}
	return &execProcess{command: command, reload: spec, done: make(chan struct{})}, nil
}

type execProcess struct {
	command *exec.Cmd
	reload  ProcessSpec
	done    chan struct{}
	once    sync.Once
	mu      sync.RWMutex
	err     error
}

func (p *execProcess) Wait() error {
	p.once.Do(func() {
		p.mu.Lock()
		p.err = p.command.Wait()
		p.mu.Unlock()
		close(p.done)
	})
	<-p.done
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.err
}

func (p *execProcess) GracefulStop(ctx context.Context) error {
	if p.command.Process == nil {
		return nil
	}
	if err := p.command.Process.Signal(os.Interrupt); err != nil {
		return err
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *execProcess) Kill() error {
	if p.command.Process == nil {
		return nil
	}
	return p.command.Process.Kill()
}

func (p *execProcess) Reload() error {
	if len(p.reload.ReloadArgs) == 0 {
		return fmt.Errorf("reload arguments are not configured")
	}
	command := exec.Command(p.reload.Binary, p.reload.ReloadArgs...)
	command.Dir = p.reload.Dir
	return command.Run()
}
