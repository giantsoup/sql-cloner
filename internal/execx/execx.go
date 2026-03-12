package execx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

type Command struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

func (c Command) String() string {
	return strings.TrimSpace(strings.Join(append([]string{c.Name}, c.Args...), " "))
}

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type StreamHandler struct {
	Stdout func(string)
	Stderr func(string)
}

type Runner interface {
	Run(context.Context, Command) (Result, error)
	Stream(context.Context, Command, StreamHandler) (Result, error)
}

type OSRunner struct{}

func NewRunner() *OSRunner {
	return &OSRunner{}
}

func (r *OSRunner) Run(ctx context.Context, cmd Command) (Result, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	execCmd := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	execCmd.Env = append(execCmd.Env, cmd.Env...)
	execCmd.Dir = cmd.Dir
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	err := execCmd.Run()
	result := Result{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if execCmd.ProcessState != nil {
		result.ExitCode = execCmd.ProcessState.ExitCode()
	}
	if err != nil {
		return result, fmt.Errorf("%s: %w", cmd.String(), err)
	}
	return result, nil
}

func (r *OSRunner) Stream(ctx context.Context, cmd Command, handler StreamHandler) (Result, error) {
	execCmd := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	execCmd.Env = append(execCmd.Env, cmd.Env...)
	execCmd.Dir = cmd.Dir

	stdoutPipe, err := execCmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := execCmd.StderrPipe()
	if err != nil {
		return Result{}, fmt.Errorf("stderr pipe: %w", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := execCmd.Start(); err != nil {
		return Result{}, fmt.Errorf("%s: %w", cmd.String(), err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	consume := func(r io.Reader, buf *bytes.Buffer, fn func(string)) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			buf.WriteString(line)
			buf.WriteByte('\n')
			if fn != nil {
				fn(line)
			}
		}
	}

	go consume(stdoutPipe, &stdout, handler.Stdout)
	go consume(stderrPipe, &stderr, handler.Stderr)

	waitErr := execCmd.Wait()
	wg.Wait()

	result := Result{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if execCmd.ProcessState != nil {
		result.ExitCode = execCmd.ProcessState.ExitCode()
	}
	if waitErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return result, ctx.Err()
		}
		return result, fmt.Errorf("%s: %w", cmd.String(), waitErr)
	}
	return result, nil
}
