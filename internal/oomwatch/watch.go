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

var (
	// DefaultCgroupPath is the default path to the cgroup directory within a
	// container. It is used to discover the cgroup files if they are not
	// provided.
	DefaultCgroupPath = "/sys/fs/cgroup/"
)

const (
	// MemoryLimitFile is the cgroup v1 memory.limit_in_bytes filepath relative
	// to DefaultCgroupPath.
	MemoryLimitFile = "memory/memory.limit_in_bytes"
	// MemoryUsageFile is the cgroup v1 memory.usage_in_bytes filepath relative
	// to DefaultCgroupPath.
	MemoryUsageFile = "memory/memory.usage_in_bytes"

	// MemoryMaxFile is the cgroup v2 memory.max filepath relative to
	// DefaultCgroupPath.
	MemoryMaxFile = "memory.max"
	// MemoryCurrentFile is the cgroup v2 memory.current filepath relative to
	// DefaultCgroupPath.
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
	memoryUsagePercentThreshold uint8
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

// New returns a new Watcher with the given configuration. If the provided
// paths are empty, it will attempt to discover the paths to the cgroup files.
// It returns an error if the paths cannot be discovered or if the provided
// configuration is invalid.
func New(memoryMaxPath, memoryCurrentPath string, memoryUsagePercentThreshold uint8, interval time.Duration, logger logr.Logger) (_ *Watcher, err error) {
	if memoryUsagePercentThreshold < 1 || memoryUsagePercentThreshold > 100 {
		return nil, fmt.Errorf("memory usage percent threshold must be between 1 and 100, got %d", memoryUsagePercentThreshold)
	}

	if minInterval := 50 * time.Millisecond; interval < minInterval {
		return nil, fmt.Errorf("interval must be at least %s, got %s", minInterval, interval)
	}

	memoryMaxPath, memoryCurrentPath, err = discoverCgroupPaths(memoryMaxPath, memoryCurrentPath)
	if err != nil {
		return nil, err
	}

	if _, err = os.Lstat(memoryCurrentPath); err != nil {
		return nil, fmt.Errorf("failed to confirm existence of current memory usage file: %w", err)
	}

	memoryMax, err := readUintFromFile(memoryMaxPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read memory usage limit: %w", err)
	}

	return &Watcher{
		memoryMax:                   memoryMax,
		memoryCurrentPath:           memoryCurrentPath,
		memoryUsagePercentThreshold: memoryUsagePercentThreshold,
		interval:                    interval,
		logger:                      logger,
	}, nil
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
			if currentPercentage >= float64(w.memoryUsagePercentThreshold) {
				w.logger.Info(fmt.Sprintf("Memory usage is near OOM (%s/%s), shutting down",
					formatSize(current), formatSize(w.memoryMax)))
				w.cancel()
				return
			}
			w.logger.V(2).Info(fmt.Sprintf("Current memory usage %s/%s (%.2f%% out of %d%%)",
				formatSize(current), formatSize(w.memoryMax), currentPercentage, w.memoryUsagePercentThreshold))
		}
	}
}

// discoverCgroupPaths attempts to automatically discover the cgroup v1 and v2
// paths for the max and current memory files when they are not provided. It
// returns the discovered and/or provided max and current paths.
// When a path is not provided and cannot be discovered, an error is returned.
func discoverCgroupPaths(memoryMaxPath, memoryCurrentPath string) (string, string, error) {
	if memoryMaxPath == "" {
		maxPathV1 := filepath.Join(DefaultCgroupPath, MemoryLimitFile)
		maxPathV2 := filepath.Join(DefaultCgroupPath, MemoryMaxFile)

		if _, err := os.Lstat(maxPathV2); err == nil {
			memoryMaxPath = maxPathV2
		} else if _, err = os.Lstat(maxPathV1); err == nil {
			memoryMaxPath = maxPathV1
		}
	}
	if memoryCurrentPath == "" {
		currentPathV1 := filepath.Join(DefaultCgroupPath, MemoryUsageFile)
		currentPathV2 := filepath.Join(DefaultCgroupPath, MemoryCurrentFile)

		if _, err := os.Lstat(currentPathV2); err == nil {
			memoryCurrentPath = currentPathV2
		} else if _, err = os.Lstat(currentPathV1); err == nil {
			memoryCurrentPath = currentPathV1
		}
	}

	if memoryMaxPath == "" && memoryCurrentPath == "" {
		return "", "", fmt.Errorf("failed to discover cgroup paths, please specify them manually")
	}
	if memoryMaxPath == "" {
		return "", "", fmt.Errorf("failed to discover max memory path, please specify it manually")
	}
	if memoryCurrentPath == "" {
		return "", "", fmt.Errorf("failed to discover current memory path, please specify it manually")
	}

	return memoryMaxPath, memoryCurrentPath, nil
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
