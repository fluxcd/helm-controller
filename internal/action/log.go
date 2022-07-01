/*
Copyright 2022 The Flux authors

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

package action

import (
	"container/ring"
	"fmt"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	helmaction "helm.sh/helm/v3/pkg/action"
)

// DefaultLogBufferSize is the default size of the LogBuffer.
const DefaultLogBufferSize = 5

// NewDebugLog returns an action.DebugLog that logs to the given logr.Logger.
func NewDebugLog(log logr.Logger) helmaction.DebugLog {
	return func(format string, v ...interface{}) {
		log.Info(fmt.Sprintf(format, v...))
	}
}

// LogBuffer is a ring buffer that logs to a Helm action.DebugLog.
type LogBuffer struct {
	mu     sync.RWMutex
	log    helmaction.DebugLog
	buffer *ring.Ring
}

// NewLogBuffer creates a new LogBuffer with the given log function
// and a buffer of the given size. If size <= 0, it defaults to
// DefaultLogBufferSize.
func NewLogBuffer(log helmaction.DebugLog, size int) *LogBuffer {
	if size <= 0 {
		size = DefaultLogBufferSize
	}
	return &LogBuffer{
		log:    log,
		buffer: ring.New(size),
	}
}

// Log adds the log message to the ring buffer before calling the actual log
// function. It is safe to call this function from multiple goroutines.
func (l *LogBuffer) Log(format string, v ...interface{}) {
	l.mu.Lock()

	// Filter out duplicate log lines, this happens for example when
	// Helm is waiting on workloads to become ready.
	msg := fmt.Sprintf(format, v...)
	if prev := l.buffer.Prev(); prev.Value != msg {
		l.buffer.Value = msg
		l.buffer = l.buffer.Next()
	}

	l.mu.Unlock()
	l.log(format, v...)
}

// Reset clears the buffer.
func (l *LogBuffer) Reset() {
	l.mu.Lock()
	l.buffer = ring.New(l.buffer.Len())
	l.mu.Unlock()
}

// String returns the contents of the buffer as a string.
func (l *LogBuffer) String() string {
	var str string
	l.mu.RLock()
	l.buffer.Do(func(s interface{}) {
		if s == nil {
			return
		}
		str += s.(string) + "\n"
	})
	l.mu.RUnlock()
	return strings.TrimSpace(str)
}
