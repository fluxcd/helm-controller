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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// stubNowTS returns a fixed time for testing purposes.
func stubNowTS() time.Time {
	return time.Date(2016, 2, 18, 12, 24, 5, 12345600, time.UTC)
}

// stubNowTS2 returns a different fixed time for testing duplicate log line timestamps.
func stubNowTS2() time.Time {
	return time.Date(2016, 2, 18, 12, 24, 6, 12345600, time.UTC)
}

func Test_logLine_String(t *testing.T) {
	ts := stubNowTS()
	ts2 := stubNowTS2()

	for _, tt := range []struct {
		name string
		line *logLine
		want string
	}{
		{
			name: "nil logLine",
			line: nil,
			want: "",
		},
		{
			name: "empty message",
			line: &logLine{ts: ts, msg: ""},
			want: "",
		},
		{
			name: "simple message",
			line: &logLine{ts: ts, msg: "test message"},
			want: fmt.Sprintf("%s: test message", ts.Format(time.RFC3339Nano)),
		},
		{
			name: "message with one duplicate",
			line: &logLine{ts: ts, lastTS: ts2, msg: "duplicate message", count: 1},
			want: fmt.Sprintf("%s: duplicate message\n%s: duplicate message", ts.Format(time.RFC3339Nano), ts2.Format(time.RFC3339Nano)),
		},
		{
			name: "message with two duplicates",
			line: &logLine{ts: ts, lastTS: ts2, msg: "duplicate message", count: 2},
			want: fmt.Sprintf("%s: duplicate message\n%s: duplicate message (1 duplicate line omitted)", ts.Format(time.RFC3339Nano), ts2.Format(time.RFC3339Nano)),
		},
		{
			name: "message with three duplicates",
			line: &logLine{ts: ts, lastTS: ts2, msg: "duplicate message", count: 3},
			want: fmt.Sprintf("%s: duplicate message\n%s: duplicate message (2 duplicate lines omitted)", ts.Format(time.RFC3339Nano), ts2.Format(time.RFC3339Nano)),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tt.line.String()).To(Equal(tt.want))
		})
	}
}

func Test_logRingBuffer_Appendf(t *testing.T) {
	origNowTS := nowTS
	defer func() { nowTS = origNowTS }()
	nowTS = stubNowTS

	t.Run("nil buffer is safe to call", func(t *testing.T) {
		g := NewWithT(t)
		var l *logRingBuffer
		g.Expect(func() { l.Appendf("test") }).NotTo(Panic())
	})

	t.Run("appends messages to buffer", func(t *testing.T) {
		g := NewWithT(t)
		ctx := context.WithValue(context.Background(), ringBufferSizeContextKey{}, 3)
		l := newLogRingBuffer(ctx)

		l.Appendf("message %d", 1)
		l.Appendf("message %d", 2)

		want := fmt.Sprintf("%[1]s: message 1\n%[1]s: message 2", stubNowTS().Format(time.RFC3339Nano))
		g.Expect(l.String()).To(Equal(want))
	})

	t.Run("handles duplicate messages", func(t *testing.T) {
		g := NewWithT(t)
		ctx := context.WithValue(context.Background(), ringBufferSizeContextKey{}, 5)
		l := newLogRingBuffer(ctx)

		l.Appendf("same message")
		l.Appendf("same message")
		l.Appendf("same message")

		want := fmt.Sprintf("%[1]s: same message\n%[1]s: same message (1 duplicate line omitted)", stubNowTS().Format(time.RFC3339Nano))
		g.Expect(l.String()).To(Equal(want))
	})

	t.Run("ring buffer wraps around", func(t *testing.T) {
		g := NewWithT(t)
		ctx := context.WithValue(context.Background(), ringBufferSizeContextKey{}, 2)
		l := newLogRingBuffer(ctx)

		l.Appendf("a")
		l.Appendf("b")
		l.Appendf("c")

		want := fmt.Sprintf("%[1]s: b\n%[1]s: c", stubNowTS().Format(time.RFC3339Nano))
		g.Expect(l.String()).To(Equal(want))
	})
}

func Test_logRingBuffer_Empty(t *testing.T) {
	t.Run("nil buffer is empty", func(t *testing.T) {
		g := NewWithT(t)
		var l *logRingBuffer
		g.Expect(l.Empty()).To(BeTrue())
	})

	t.Run("new buffer is empty", func(t *testing.T) {
		g := NewWithT(t)
		ctx := context.WithValue(context.Background(), ringBufferSizeContextKey{}, 5)
		l := newLogRingBuffer(ctx)
		g.Expect(l.Empty()).To(BeTrue())
	})

	t.Run("buffer with entries is not empty", func(t *testing.T) {
		g := NewWithT(t)
		ctx := context.WithValue(context.Background(), ringBufferSizeContextKey{}, 5)
		l := newLogRingBuffer(ctx)
		l.Appendf("test message")
		g.Expect(l.Empty()).To(BeFalse())
	})
}

func Test_logRingBuffer_String(t *testing.T) {
	origNowTS := nowTS
	defer func() { nowTS = origNowTS }()
	nowTS = stubNowTS

	t.Run("nil buffer returns empty string", func(t *testing.T) {
		g := NewWithT(t)
		var l *logRingBuffer
		g.Expect(l.String()).To(Equal(""))
	})

	t.Run("empty buffer returns empty string", func(t *testing.T) {
		g := NewWithT(t)
		ctx := context.WithValue(context.Background(), ringBufferSizeContextKey{}, 5)
		l := newLogRingBuffer(ctx)
		g.Expect(l.String()).To(Equal(""))
	})

	t.Run("returns all messages joined by newlines", func(t *testing.T) {
		g := NewWithT(t)
		ctx := context.WithValue(context.Background(), ringBufferSizeContextKey{}, 5)
		l := newLogRingBuffer(ctx)

		l.Appendf("first")
		l.Appendf("second")
		l.Appendf("third")

		want := fmt.Sprintf("%[1]s: first\n%[1]s: second\n%[1]s: third", stubNowTS().Format(time.RFC3339Nano))
		g.Expect(l.String()).To(Equal(want))
	})

	t.Run("handles mixed duplicates and unique messages", func(t *testing.T) {
		g := NewWithT(t)
		ctx := context.WithValue(context.Background(), ringBufferSizeContextKey{}, 10)
		l := newLogRingBuffer(ctx)

		l.Appendf("a")
		l.Appendf("b")
		l.Appendf("b")
		l.Appendf("b")
		l.Appendf("c")
		l.Appendf("c")

		want := fmt.Sprintf("%[1]s: a\n%[1]s: b\n%[1]s: b (1 duplicate line omitted)\n%[1]s: c\n%[1]s: c", stubNowTS().Format(time.RFC3339Nano))
		g.Expect(l.String()).To(Equal(want))
	})
}

func TestLogBuffer_Enabled(t *testing.T) {
	g := NewWithT(t)

	ctx := log.IntoContext(context.Background(), logr.Discard())
	l := newLogBuffer(ctx, 0)

	g.Expect(l.Enabled(ctx, slog.LevelDebug)).To(BeTrue())
	g.Expect(l.Enabled(ctx, slog.LevelInfo)).To(BeTrue())
	g.Expect(l.Enabled(ctx, slog.LevelWarn)).To(BeTrue())
	g.Expect(l.Enabled(ctx, slog.LevelError)).To(BeTrue())
}

func TestLogBuffer_Handle(t *testing.T) {
	origNowTS := nowTS
	defer func() { nowTS = origNowTS }()
	nowTS = stubNowTS

	t.Run("handles info level message", func(t *testing.T) {
		g := NewWithT(t)

		ctx := log.IntoContext(context.Background(), logr.Discard())
		l := NewDebugLogBuffer(ctx)

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "info message", 0)
		err := l.Handle(ctx, record)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(l.String()).To(ContainSubstring("info message"))
	})

	t.Run("handles error level message with prefix", func(t *testing.T) {
		g := NewWithT(t)

		ctx := log.IntoContext(context.Background(), logr.Discard())
		l := NewDebugLogBuffer(ctx)

		record := slog.NewRecord(time.Now(), slog.LevelError, "error occurred", 0)
		err := l.Handle(ctx, record)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(l.String()).To(ContainSubstring("error: error occurred"))
	})

	t.Run("handles warning level message with prefix", func(t *testing.T) {
		g := NewWithT(t)

		ctx := log.IntoContext(context.Background(), logr.Discard())
		l := NewDebugLogBuffer(ctx)

		record := slog.NewRecord(time.Now(), slog.LevelWarn, "warning issued", 0)
		err := l.Handle(ctx, record)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(l.String()).To(ContainSubstring("warning: warning issued"))
	})

	t.Run("handles message with attributes", func(t *testing.T) {
		g := NewWithT(t)

		ctx := log.IntoContext(context.Background(), logr.Discard())
		l := NewDebugLogBuffer(ctx)

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
		record.AddAttrs(slog.String("key", "value"))
		err := l.Handle(ctx, record)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(l.String()).To(ContainSubstring(`"key":"value"`))
	})
}

func TestLogBuffer_WithAttrs(t *testing.T) {
	g := NewWithT(t)

	ctx := log.IntoContext(context.Background(), logr.Discard())
	l := NewDebugLogBuffer(ctx)

	handler := l.WithAttrs([]slog.Attr{slog.String("attr1", "val1")})
	g.Expect(handler).ToNot(BeNil())

	lb, ok := handler.(*LogBuffer)
	g.Expect(ok).To(BeTrue())
	g.Expect(lb.attrs).To(HaveLen(1))
	g.Expect(lb.attrs[0].attr.Key).To(Equal("attr1"))
	g.Expect(lb.attrs[0].attr.Value.String()).To(Equal("val1"))
}

func TestLogBuffer_WithGroup(t *testing.T) {
	t.Run("returns same handler for empty group name", func(t *testing.T) {
		g := NewWithT(t)

		ctx := log.IntoContext(context.Background(), logr.Discard())
		l := NewDebugLogBuffer(ctx)

		handler := l.WithGroup("")
		g.Expect(handler).To(Equal(l))
	})

	t.Run("creates new handler with group", func(t *testing.T) {
		g := NewWithT(t)

		ctx := log.IntoContext(context.Background(), logr.Discard())
		l := NewDebugLogBuffer(ctx)

		handler := l.WithGroup("mygroup")
		g.Expect(handler).ToNot(Equal(l))

		lb, ok := handler.(*LogBuffer)
		g.Expect(ok).To(BeTrue())
		g.Expect(lb.group).To(Equal([]string{"mygroup"}))
	})

	t.Run("supports nested groups", func(t *testing.T) {
		g := NewWithT(t)

		ctx := log.IntoContext(context.Background(), logr.Discard())
		l := NewDebugLogBuffer(ctx)

		handler := l.WithGroup("outer").WithGroup("inner")
		lb, ok := handler.(*LogBuffer)
		g.Expect(ok).To(BeTrue())
		g.Expect(lb.group).To(Equal([]string{"outer", "inner"}))
	})
}

func TestLogBuffer_HandleWithMixedAttrsAndGroups(t *testing.T) {
	origNowTS := nowTS
	defer func() { nowTS = origNowTS }()
	nowTS = stubNowTS

	t.Run("attrs before group remain ungrouped", func(t *testing.T) {
		g := NewWithT(t)

		ctx := log.IntoContext(context.Background(), logr.Discard())
		l := NewDebugLogBuffer(ctx)

		// Add attr first, then group with its own attr
		handler := l.WithAttrs([]slog.Attr{slog.String("root", "val1")}).
			WithGroup("nested").
			WithAttrs([]slog.Attr{slog.String("inner", "val2")})

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "mixed test", 0)
		lb, ok := handler.(*LogBuffer)
		g.Expect(ok).To(BeTrue())

		err := lb.Handle(ctx, record)
		g.Expect(err).ToNot(HaveOccurred())

		output := lb.String()
		g.Expect(output).To(ContainSubstring("mixed test"))

		// Extract and parse JSON from output
		jsonStart := strings.Index(output, "{")
		g.Expect(jsonStart).To(BeNumerically(">=", 0), "expected JSON in output")
		var attrs map[string]any
		err = json.Unmarshal([]byte(output[jsonStart:]), &attrs)
		g.Expect(err).ToNot(HaveOccurred())

		// root attr should be at top level
		g.Expect(attrs).To(HaveKeyWithValue("root", "val1"))
		// inner attr should be nested under "nested" group
		g.Expect(attrs).To(HaveKey("nested"))
		nested, ok := attrs["nested"].(map[string]any)
		g.Expect(ok).To(BeTrue(), "nested should be a map")
		g.Expect(nested).To(HaveKeyWithValue("inner", "val2"))
	})

	t.Run("alternating attrs and groups", func(t *testing.T) {
		g := NewWithT(t)

		ctx := log.IntoContext(context.Background(), logr.Discard())
		l := NewDebugLogBuffer(ctx)

		// Create a chain: attr -> group -> attr -> group -> attr
		handler := l.
			WithAttrs([]slog.Attr{slog.String("level0", "a")}).
			WithGroup("g1").
			WithAttrs([]slog.Attr{slog.String("level1", "b")}).
			WithGroup("g2").
			WithAttrs([]slog.Attr{slog.String("level2", "c")})

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "alternating test", 0)
		lb, ok := handler.(*LogBuffer)
		g.Expect(ok).To(BeTrue())

		err := lb.Handle(ctx, record)
		g.Expect(err).ToNot(HaveOccurred())

		output := lb.String()
		g.Expect(output).To(ContainSubstring("alternating test"))

		// Extract and parse JSON from output
		jsonStart := strings.Index(output, "{")
		g.Expect(jsonStart).To(BeNumerically(">=", 0), "expected JSON in output")
		var attrs map[string]any
		err = json.Unmarshal([]byte(output[jsonStart:]), &attrs)
		g.Expect(err).ToNot(HaveOccurred())

		// level0 should be at root
		g.Expect(attrs).To(HaveKeyWithValue("level0", "a"))

		// level1 should be under g1
		g.Expect(attrs).To(HaveKey("g1"))
		g1, ok := attrs["g1"].(map[string]any)
		g.Expect(ok).To(BeTrue(), "g1 should be a map")
		g.Expect(g1).To(HaveKeyWithValue("level1", "b"))

		// level2 should be under g1.g2
		g.Expect(g1).To(HaveKey("g2"))
		g2, ok := g1["g2"].(map[string]any)
		g.Expect(ok).To(BeTrue(), "g2 should be a map")
		g.Expect(g2).To(HaveKeyWithValue("level2", "c"))
	})
}

func TestNewTraceLogger(t *testing.T) {
	g := NewWithT(t)

	ctx := log.IntoContext(context.Background(), logr.Discard())
	handler := NewTraceLogger(ctx)

	g.Expect(handler).ToNot(BeNil())

	lb, ok := handler.(*LogBuffer)
	g.Expect(ok).To(BeTrue())
	g.Expect(lb.buf).To(BeNil())
}

func TestNewDebugLogBuffer(t *testing.T) {
	g := NewWithT(t)

	ctx := log.IntoContext(context.Background(), logr.Discard())
	handler := NewDebugLogBuffer(ctx)

	g.Expect(handler).ToNot(BeNil())
	g.Expect(handler.buf).ToNot(BeNil())
}

func Test_newLogRingBuffer_defaultSize(t *testing.T) {
	g := NewWithT(t)

	ctx := context.Background()
	l := newLogRingBuffer(ctx)

	g.Expect(l).ToNot(BeNil())
	g.Expect(l.buf.Len()).To(Equal(10))
}

func Test_newLogRingBuffer_customSize(t *testing.T) {
	g := NewWithT(t)

	ctx := context.WithValue(context.Background(), ringBufferSizeContextKey{}, 20)
	l := newLogRingBuffer(ctx)

	g.Expect(l).ToNot(BeNil())
	g.Expect(l.buf.Len()).To(Equal(20))
}
