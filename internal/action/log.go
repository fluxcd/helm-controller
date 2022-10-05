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
	"time"

	"github.com/go-logr/logr"
	helmaction "helm.sh/helm/v3/pkg/action"
)

// DefaultLogBufferSize is the default size of the LogBuffer.
const DefaultLogBufferSize = 5

// nowTS can be used to stub out time.Now() in tests.
var nowTS = time.Now

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

// logLine is a log message with a timestamp.
type logLine struct {
	ts     time.Time
	lastTS time.Time
	msg    string
	count  int64
}

// String returns the log line as a string, in the format of:
// '<RFC3339 nano timestamp>: <message>'. But only if the message is not empty.
func (l *logLine) String() string {
	if l == nil || l.msg == "" {
		return ""
	}

	msg := fmt.Sprintf("%s %s", l.ts.Format(time.RFC3339Nano), l.msg)
	if c := l.count; c > 0 {
		msg += fmt.Sprintf("\n%s %s", l.lastTS.Format(time.RFC3339Nano), l.msg)
	}
	if c := l.count - 1; c > 0 {
		var dup = "line"
		if c > 1 {
			dup += "s"
		}
		msg += fmt.Sprintf(" (%d duplicate %s omitted)", c, dup)
	}
	return msg
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
	prev, ok := l.buffer.Prev().Value.(*logLine)
	if ok && prev.msg == msg {
		prev.count++
		prev.lastTS = nowTS().UTC()
		l.buffer.Prev().Value = prev
	}
	if !ok || prev.msg != msg {
		l.buffer.Value = &logLine{
			ts:  nowTS().UTC(),
			msg: msg,
		}
		l.buffer = l.buffer.Next()
	}

	l.mu.Unlock()
	l.log(format, v...)
}

// Len returns the count of non-empty values in the buffer.
func (l *LogBuffer) Len() (count int) {
	l.mu.RLock()
	l.buffer.Do(func(s interface{}) {
		if s == nil {
			return
		}
		ll, ok := s.(*logLine)
		if !ok || ll.String() == "" {
			return
		}
		count++
	})
	l.mu.RUnlock()
	return
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
		ll, ok := s.(*logLine)
		if !ok {
			return
		}
		if msg := ll.String(); msg != "" {
			str += msg + "\n"
		}
	})
	l.mu.RUnlock()
	return strings.TrimSpace(str)
}
