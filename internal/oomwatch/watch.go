/*
Copyright 2023 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package oomwatch provides a way to detect near OOM conditions.
package oomwatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

const (
	// DefaultCgroupPath is the default path to the cgroup directory.
	DefaultCgroupPath = "/sys/fs/cgroup/"
	// MemoryMaxFile is the cgroup memory.max filename.
	MemoryMaxFile = "memory.max"
	// MemoryCurrentFile is the cgroup memory.current filename.
	MemoryCurrentFile = "memory.current"
)

// Watcher can be used to detect near OOM conditions.
type Watcher struct {
	// memoryMax is the maximum amount of memory that can be used by the system.
	memoryMax uint64
	// memoryCurrentPath is the cgroup memory.current filepath.
	memoryCurrentPath string
	// memoryUsagePercentThreshold is the threshold at which the system is
	// considered to be near OOM.
	memoryUsagePercentThreshold float64
	// interval is the interval at which to check for OOM.
	interval time.Duration
	// logger is the logger to use.
	logger logr.Logger

	// ctx is the context that is canceled when OOM is detected.
	ctx context.Context
	// cancel is the function that cancels the context.
	cancel context.CancelFunc
	// once is used to ensure that Watch is only called once.
	once sync.Once
}

// New returns a new Watcher.
func New(memoryMaxPath, memoryCurrentPath string, memoryUsagePercentThreshold float64, interval time.Duration, logger logr.Logger) (*Watcher, error) {
	if memoryUsagePercentThreshold < 1 || memoryUsagePercentThreshold > 100 {
		return nil, fmt.Errorf("memory usage percent threshold must be between 1 and 100, got %f", memoryUsagePercentThreshold)
	}

	if _, err := os.Lstat(memoryCurrentPath); err != nil {
		return nil, fmt.Errorf("failed to stat %q: %w", memoryCurrentPath, err)
	}

	memoryMax, err := readUintFromFile(memoryMaxPath)
	if err != nil {
		return nil, err
	}

	return &Watcher{
		memoryMax:                   memoryMax,
		memoryCurrentPath:           memoryCurrentPath,
		memoryUsagePercentThreshold: memoryUsagePercentThreshold,
		interval:                    interval,
		logger:                      logger,
	}, nil
}

// NewDefault returns a new Watcher with default path values.
func NewDefault(memoryUsagePercentThreshold float64, interval time.Duration, logger logr.Logger) (*Watcher, error) {
	return New(
		filepath.Join(DefaultCgroupPath, MemoryMaxFile),
		filepath.Join(DefaultCgroupPath, MemoryCurrentFile),
		memoryUsagePercentThreshold,
		interval,
		logger,
	)
}

// Watch returns a context that is canceled when the system reaches the
// configured memory usage threshold. Calling Watch multiple times will return
// the same context.
func (w *Watcher) Watch(ctx context.Context) context.Context {
	w.once.Do(func() {
		w.ctx, w.cancel = context.WithCancel(ctx)
		go w.watchForNearOOM(ctx)
	})
	return w.ctx
}

// watchForNearOOM polls the memory.current file on the configured interval
// and cancels the context within Watcher when the system is near OOM.
// It is expected that this function is called in a goroutine. Canceling
// provided context will cause the goroutine to exit.
func (w *Watcher) watchForNearOOM(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Shutdown signal received, stopping watch for near OOM")
			return
		case <-t.C:
			current, err := readUintFromFile(w.memoryCurrentPath)
			if err != nil {
				w.logger.Error(err, "Failed to read current memory usage, skipping check")
				continue
			}

			currentPercentage := float64(current) / float64(w.memoryMax) * 100
			if currentPercentage >= w.memoryUsagePercentThreshold {
				w.logger.Info(fmt.Sprintf("Memory usage is near OOM (%s/%s), shutting down",
					formatSize(current), formatSize(w.memoryMax)))
				w.cancel()
				return
			}
			w.logger.V(2).Info(fmt.Sprintf("Current memory usage %s/%s (%.2f%% out of %.2f%%)",
				formatSize(current), formatSize(w.memoryMax), currentPercentage, w.memoryUsagePercentThreshold))
		}
	}
}

// readUintFromFile reads an uint64 from the file at the given path.
func readUintFromFile(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
}

// formatSize formats the given size in bytes to a human-readable format.
func formatSize(b uint64) string {
	if b == 0 {
		return "-"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB",
		float64(b)/float64(div), "KMGTPE"[exp])
}
