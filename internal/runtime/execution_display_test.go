package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

type failingWriter struct{ calls int }

func (w *failingWriter) Write([]byte) (int, error) { w.calls++; return 0, errors.New("broken pipe") }

// A failed write of the last displayed fatal is the host's output failure,
// not silently replaced by the PHP fatal.
func TestDisplayedFatalWriterFailureIsReported(t *testing.T) {
	for _, inHandler := range []bool{false, true} {
		r := readyExecutionRequest(t)
		w := &failingWriter{}
		x := newTestExecution(t, r, 64, w, executionLimits{32, 64, 4096})
		x.errors = errorChannels{display: true, log: true}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		result := x.run(ctx, func(x *execution) completion {
			if inHandler {
				if err := x.startBuffer("{closure:a.php:1}", func(_ *execution, _ []byte) ([]byte, completion) {
					return nil, completion{kind: endFatal, err: errors.New("boom")}
				}); err != nil {
					t.Fatal(err)
				}
				return x.endBuffer("ob_end_flush")
			}
			return completion{kind: endFatal, err: errors.New("boom")}
		})
		cancel()
		if w.calls == 0 || result.end.kind != endOutput {
			t.Errorf("inHandler=%v: end %v after %d writes, want the writer failure", inHandler, result.end.kind, w.calls)
		}
	}
}
