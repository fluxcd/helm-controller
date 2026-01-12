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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/fluxcd/pkg/runtime/logger"
)

// nowTS can be used to stub out time.Now() in tests.
var nowTS = time.Now

// NewTraceLogger returns an slog.Handler that logs to the logger from the context at Trace level.
func NewTraceLogger(ctx context.Context) slog.Handler {
	return newLogBuffer(ctx, logger.TraceLevel)
}

// NewDebugLogBuffer returns an slog.Handler that logs to the logger from the context at Debug level,
// and also keeps a ring buffer of log messages.
func NewDebugLogBuffer(ctx context.Context) *LogBuffer {
	l := newLogBuffer(ctx, logger.DebugLevel)
	l.buf = newLogRingBuffer(ctx)
	return l
}

// LogBuffer implements slog.Handler by logging to a
// logr.Logger calling log.Info, and to a ring buffer
// if level is Debug.
type LogBuffer struct {
	attrs []groupedAttr
	group []string

	// destinations
	log logr.Logger
	buf *logRingBuffer
}

// groupedAttr is an slog.Attr belonging to a group.
type groupedAttr struct {
	group []string
	attr  slog.Attr
}

// newLogBuffer creates a new LogBuffer.
func newLogBuffer(ctx context.Context, level int) *LogBuffer {
	return &LogBuffer{log: log.FromContext(ctx).V(level)}
}

// Appendf adds the log message to the ring buffer.
func (l *LogBuffer) Appendf(format string, v ...any) {
	if l != nil {
		l.buf.Appendf(format, v...)
	}
}

// Empty returns true if the buffer is empty.
func (l *LogBuffer) Empty() bool {
	return l == nil || l.buf.Empty()
}

// String returns the contents of the buffer as a string.
func (l *LogBuffer) String() string {
	if l == nil {
		return ""
	}
	return l.buf.String()
}

// Enabled implements slog.Handler.
func (l *LogBuffer) Enabled(context.Context, slog.Level) bool {
	// We handle the level on the logr.Logger side.
	return true
}

// Handle implements slog.Handler.
func (l *LogBuffer) Handle(_ context.Context, r slog.Record) error {
	// Prepare message based on the record level.
	var msg string
	switch r.Level {
	case slog.LevelError:
		msg = fmt.Sprintf("error: %s", r.Message)
	case slog.LevelWarn:
		msg = fmt.Sprintf("warning: %s", r.Message)
	default:
		msg = r.Message
	}

	// Collect record attributes.
	slogAttrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		slogAttrs = append(slogAttrs, a)
		return true
	})
	l = l.withAttrs(slogAttrs) // We intentionally update the method receiver here (it doesn't mutate the original).

	// Build nested attribute map.
	attrs := make(map[string]any)
	for _, ga := range l.attrs {
		target := attrs
		for _, g := range ga.group {
			next, ok := target[g].(map[string]any)
			if !ok {
				node := make(map[string]any)
				target[g] = node
				next = node
			}
			target = next
		}
		target[ga.attr.Key] = ga.attr.Value.Any()
	}

	// Sink to logger.
	keysAndValues := make([]any, 0, len(attrs)*2)
	for k, v := range attrs {
		keysAndValues = append(keysAndValues, k, v)
	}
	l.log.Info(msg, keysAndValues...)

	// Sink to buffer.
	b, err := json.Marshal(attrs)
	if err != nil {
		l.buf.Appendf("%s", msg)
		return err
	}
	l.buf.Appendf("%s: %s", msg, string(b))
	return nil
}

// WithAttrs implements slog.Handler.
func (l *LogBuffer) WithAttrs(attrs []slog.Attr) slog.Handler {
	return l.withAttrs(attrs)
}
func (l *LogBuffer) withAttrs(attrs []slog.Attr) *LogBuffer {
	nl := *l
	nl.attrs = make([]groupedAttr, 0, len(l.attrs)+len(attrs))
	nl.attrs = append(nl.attrs, l.attrs...)
	for _, attr := range attrs {
		nl.attrs = append(nl.attrs, groupedAttr{
			group: l.group,
			attr:  attr,
		})
	}
	return &nl
}

// WithGroup implements slog.Handler.
func (l *LogBuffer) WithGroup(name string) slog.Handler {
	if name == "" {
		return l
	}
	nl := *l
	nl.group = make([]string, 0, len(l.group)+1)
	nl.group = append(nl.group, l.group...)
	nl.group = append(nl.group, name)
	return &nl
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

	msg := fmt.Sprintf("%s: %s", l.ts.Format(time.RFC3339Nano), l.msg)
	if c := l.count; c > 0 {
		msg += fmt.Sprintf("\n%s: %s", l.lastTS.Format(time.RFC3339Nano), l.msg)
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

// logRingBuffer is a ring buffer for logLine entries.
type logRingBuffer struct {
	buf *ring.Ring
	mu  sync.RWMutex
}

// ringBufferSizeContextKey is the context key for the ring buffer size.
// Used only for testing logRingBuffer.
type ringBufferSizeContextKey struct{}

// newLogRingBuffer creates a new logRingBuffer that logs to the logger from the context at Debug level.
func newLogRingBuffer(ctx context.Context) *logRingBuffer {
	size := 10
	if v := ctx.Value(ringBufferSizeContextKey{}); v != nil {
		size = v.(int)
	}
	return &logRingBuffer{buf: ring.New(size)}
}

// Appendf adds the log message to the ring buffer before calling the actual log
// function. It is safe to call this function from multiple goroutines.
func (l *logRingBuffer) Appendf(format string, v ...any) {
	if l == nil {
		return
	}

	l.mu.Lock()

	// Filter out duplicate log lines, this happens for example when
	// Helm is waiting on workloads to become ready.
	msg := fmt.Sprintf(format, v...)
	prev, ok := l.buf.Prev().Value.(*logLine)
	if ok && prev.msg == msg {
		prev.count++
		prev.lastTS = nowTS().UTC()
		l.buf.Prev().Value = prev
	}
	if !ok || prev.msg != msg {
		l.buf.Value = &logLine{
			ts:  nowTS().UTC(),
			msg: msg,
		}
		l.buf = l.buf.Next()
	}

	l.mu.Unlock()
}

// Empty returns true if the buffer is empty.
func (l *logRingBuffer) Empty() bool {
	if l == nil {
		return true
	}

	var count int
	l.mu.RLock()
	l.buf.Do(func(s any) {
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
	return count == 0
}

// String returns the contents of the buffer as a string.
func (l *logRingBuffer) String() string {
	if l == nil {
		return ""
	}

	var str string
	l.mu.RLock()
	l.buf.Do(func(s any) {
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
