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

package oomwatch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
)

func TestNew(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		g := NewWithT(t)

		mockMemoryMax := filepath.Join(t.TempDir(), MemoryMaxFile)
		g.Expect(os.WriteFile(mockMemoryMax, []byte("1000000000"), 0o640)).To(Succeed())

		mockMemoryCurrent := filepath.Join(t.TempDir(), MemoryCurrentFile)
		_, err := os.Create(mockMemoryCurrent)
		g.Expect(err).ToNot(HaveOccurred())

		w, err := New(mockMemoryMax, mockMemoryCurrent, 1, time.Second, logr.Discard())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(w).To(BeEquivalentTo(&Watcher{
			memoryMax:                   uint64(1000000000),
			memoryCurrentPath:           mockMemoryCurrent,
			memoryUsagePercentThreshold: 1,
			interval:                    time.Second,
			logger:                      logr.Discard(),
		}))
	})

	t.Run("auto discovery", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			g := NewWithT(t)

			setDefaultCgroupPath(t)

			mockMemoryMax := filepath.Join(DefaultCgroupPath, MemoryMaxFile)
			g.Expect(os.WriteFile(mockMemoryMax, []byte("1000000000"), 0o640)).To(Succeed())

			mockMemoryCurrent := filepath.Join(DefaultCgroupPath, MemoryCurrentFile)
			_, err := os.Create(mockMemoryCurrent)
			g.Expect(err).ToNot(HaveOccurred())

			w, err := New("", "", 1, time.Second, logr.Discard())
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(w).To(BeEquivalentTo(&Watcher{
				memoryMax:                   uint64(1000000000),
				memoryCurrentPath:           mockMemoryCurrent,
				memoryUsagePercentThreshold: 1,
				interval:                    time.Second,
				logger:                      logr.Discard(),
			}))
		})

		t.Run("failure", func(t *testing.T) {
			g := NewWithT(t)

			setDefaultCgroupPath(t)

			_, err := New("", "", 1, time.Second, logr.Discard())
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring("failed to discover cgroup paths"))
		})
	})

	t.Run("validation", func(t *testing.T) {
		t.Run("memory usage percentage threshold", func(t *testing.T) {
			t.Run("less than 1", func(t *testing.T) {
				g := NewWithT(t)

				_, err := New("", "", 0, 0, logr.Discard())
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError("memory usage percent threshold must be between 1 and 100, got 0"))
			})
			t.Run("greater than 100", func(t *testing.T) {
				g := NewWithT(t)

				_, err := New("", "", 101, 0, logr.Discard())
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError("memory usage percent threshold must be between 1 and 100, got 101"))
			})
		})

		t.Run("interval", func(t *testing.T) {
			t.Run("less than 50ms", func(t *testing.T) {
				g := NewWithT(t)

				_, err := New("", "", 1, 49*time.Millisecond, logr.Discard())
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError("interval must be at least 50ms, got 49ms"))
			})
		})

		t.Run("memory current path", func(t *testing.T) {
			t.Run("does not exist", func(t *testing.T) {
				g := NewWithT(t)

				_, err := New("ignore", "does.not.exist", 1, 50*time.Second, logr.Discard())
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(`failed to confirm existence of current memory usage file: lstat does.not.exist: no such file or directory`))
			})
		})

		t.Run("memory max path", func(t *testing.T) {
			t.Run("does not exist", func(t *testing.T) {
				g := NewWithT(t)

				mockMemoryCurrent := filepath.Join(t.TempDir(), MemoryMaxFile)
				_, err := os.Create(mockMemoryCurrent)
				g.Expect(err).NotTo(HaveOccurred())

				_, err = New("does.not.exist", mockMemoryCurrent, 1, 50*time.Second, logr.Discard())
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(`failed to read memory usage limit: open does.not.exist: no such file or directory`))
			})
		})
	})
}

func TestWatcher_Watch(t *testing.T) {
	t.Run("returns same context", func(t *testing.T) {
		g := NewWithT(t)

		mockMemoryMax := filepath.Join(t.TempDir(), MemoryMaxFile)
		g.Expect(os.WriteFile(mockMemoryMax, []byte("1000000000"), 0o640)).To(Succeed())

		mockMemoryCurrent := filepath.Join(t.TempDir(), MemoryCurrentFile)
		_, err := os.Create(mockMemoryCurrent)
		g.Expect(err).ToNot(HaveOccurred())

		w, err := New(mockMemoryMax, mockMemoryCurrent, 1, time.Second, logr.Discard())
		g.Expect(err).ToNot(HaveOccurred())

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		g.Expect(w.Watch(ctx)).To(Equal(w.Watch(ctx)))
	})

	t.Run("cancels context when memory usage is above threshold", func(t *testing.T) {
		g := NewWithT(t)

		mockMemoryCurrent := filepath.Join(t.TempDir(), MemoryCurrentFile)
		g.Expect(os.WriteFile(mockMemoryCurrent, []byte("1000000000"), 0o640)).To(Succeed())

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		w := &Watcher{
			memoryMax:                   uint64(1000000000),
			memoryCurrentPath:           mockMemoryCurrent,
			memoryUsagePercentThreshold: 95,
			interval:                    10 * time.Millisecond,
			logger:                      logr.Discard(),
			ctx:                         ctx,
			cancel:                      cancel,
		}

		go func() {
			<-w.ctx.Done()
			g.Expect(w.ctx.Err()).To(MatchError(context.Canceled))
		}()
	})
}

func TestWatcher_watchForNearOOM(t *testing.T) {
	t.Run("does not cancel context when memory usage is below threshold", func(t *testing.T) {
		g := NewWithT(t)

		mockMemoryCurrent := filepath.Join(t.TempDir(), MemoryCurrentFile)
		g.Expect(os.WriteFile(mockMemoryCurrent, []byte("940000000"), 0o640)).To(Succeed())

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		w := &Watcher{
			memoryMax:                   uint64(1000000000),
			memoryCurrentPath:           mockMemoryCurrent,
			memoryUsagePercentThreshold: 95,
			interval:                    500 * time.Millisecond,
			logger:                      logr.Discard(),
			ctx:                         ctx,
			cancel:                      cancel,
		}

		innerCtx, innerCancel := context.WithCancel(context.Background())
		go w.watchForNearOOM(innerCtx)

		select {
		case <-ctx.Done():
			t.Fatal("context should not have been cancelled")
		case <-time.After(1 * time.Second):
			// This also tests if the inner context stops the watcher.
			innerCancel()
		}
	})

	t.Run("cancels context when memory usage is above threshold", func(t *testing.T) {
		g := NewWithT(t)

		mockMemoryCurrent := filepath.Join(t.TempDir(), MemoryCurrentFile)
		g.Expect(os.WriteFile(mockMemoryCurrent, []byte("0"), 0o640)).To(Succeed())

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		w := &Watcher{
			memoryMax:                   uint64(1000000000),
			memoryCurrentPath:           mockMemoryCurrent,
			memoryUsagePercentThreshold: 95,
			interval:                    500 * time.Millisecond,
			logger:                      logr.Discard(),
			ctx:                         ctx,
			cancel:                      cancel,
		}

		go w.watchForNearOOM(context.TODO())

		select {
		case <-ctx.Done():
		case <-time.After(500 * time.Millisecond):
			g.Expect(os.WriteFile(mockMemoryCurrent, []byte("950000001"), 0o640)).To(Succeed())
		case <-time.After(2 * time.Second):
			t.Fatal("context was not cancelled")
		}
	})

	t.Run("continues to attempt to read memory.current", func(t *testing.T) {
		g := NewWithT(t)

		mockMemoryCurrent := filepath.Join(t.TempDir(), MemoryCurrentFile)
		g.Expect(os.WriteFile(mockMemoryCurrent, []byte("0"), 0o000)).To(Succeed())

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		w := &Watcher{
			memoryMax:                   uint64(1000000000),
			memoryCurrentPath:           mockMemoryCurrent,
			memoryUsagePercentThreshold: 95,
			interval:                    500 * time.Millisecond,
			logger:                      logr.Discard(),
			ctx:                         ctx,
			cancel:                      cancel,
		}

		go w.watchForNearOOM(context.TODO())

		var readable bool
		select {
		case <-ctx.Done():
			if !readable {
				t.Fatal("context was cancelled before memory.current was readable")
			}
		case <-time.After(1 * time.Second):
			g.Expect(os.Chmod(mockMemoryCurrent, 0o640)).To(Succeed())
			g.Expect(os.WriteFile(mockMemoryCurrent, []byte("950000001"), 0o640)).To(Succeed())
			readable = true
		case <-time.After(2 * time.Second):
			t.Fatal("context was not cancelled")
		}
	})
}

func Test_discoverCgroupPaths(t *testing.T) {
	t.Run("discovers memory max path", func(t *testing.T) {
		paths := []string{
			MemoryMaxFile,
			MemoryLimitFile,
		}
		for _, p := range paths {
			t.Run(p, func(t *testing.T) {
				g := NewWithT(t)

				setDefaultCgroupPath(t)

				maxPathMock := filepath.Join(DefaultCgroupPath, p)
				g.Expect(os.MkdirAll(filepath.Dir(maxPathMock), 0o755)).To(Succeed())
				g.Expect(os.WriteFile(maxPathMock, []byte("0"), 0o640)).To(Succeed())

				currentDummy := filepath.Join(DefaultCgroupPath, "dummy")
				max, current, err := discoverCgroupPaths("", currentDummy)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(max).To(Equal(maxPathMock))
				g.Expect(current).To(Equal(currentDummy))
			})
		}
	})

	t.Run("discovers memory current path", func(t *testing.T) {
		paths := []string{
			MemoryCurrentFile,
			MemoryUsageFile,
		}
		for _, p := range paths {
			t.Run(p, func(t *testing.T) {
				g := NewWithT(t)

				setDefaultCgroupPath(t)

				currentPathMock := filepath.Join(DefaultCgroupPath, p)
				g.Expect(os.MkdirAll(filepath.Dir(currentPathMock), 0o755)).To(Succeed())
				g.Expect(os.WriteFile(currentPathMock, []byte("0"), 0o640)).To(Succeed())

				maxDummy := filepath.Join(DefaultCgroupPath, "dummy")
				max, current, err := discoverCgroupPaths(maxDummy, "")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(max).To(Equal(maxDummy))
				g.Expect(current).To(Equal(currentPathMock))
			})
		}
	})

	t.Run("returns provided paths", func(t *testing.T) {
		g := NewWithT(t)

		maxDummy := filepath.Join(DefaultCgroupPath, "dummy")
		currentDummy := filepath.Join(DefaultCgroupPath, "dummy")

		max, current, err := discoverCgroupPaths(maxDummy, currentDummy)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(max).To(Equal(maxDummy))
		g.Expect(current).To(Equal(currentDummy))
	})

	t.Run("returns error when no paths are discovered", func(t *testing.T) {
		g := NewWithT(t)

		setDefaultCgroupPath(t)

		max, min, err := discoverCgroupPaths("", "")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to discover cgroup paths"))
		g.Expect(max).To(BeEmpty())
		g.Expect(min).To(BeEmpty())
	})
}

func setDefaultCgroupPath(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		reset := DefaultCgroupPath
		DefaultCgroupPath = reset
	})
	DefaultCgroupPath = t.TempDir()
}
