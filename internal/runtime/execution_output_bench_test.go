package runtime

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// BenchmarkOutputWrite measures echo into nested output buffers, where the
// output limit is checked on every write.
func BenchmarkOutputWrite(b *testing.B) {
	for _, depth := range []int{1, 3, 8} {
		b.Run(fmt.Sprint(depth), func(b *testing.B) {
			r, _ := NewRequest(nil)
			if err := r.Admit(func(context.Context) error { return nil }); err != nil {
				b.Fatal(err)
			}
			if err := r.BeginInit(); err != nil {
				b.Fatal(err)
			}
			if err := r.Ready(); err != nil {
				b.Fatal(err)
			}
			x, err := newExecution(context.Background(), r, 64, nil, executionLimits{32, 64, 1 << 40})
			if err != nil {
				b.Fatal(err)
			}
			cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			chunk := []byte("x")
			x.run(cleanup, func(x *execution) completion {
				for i := 0; i < depth; i++ {
					if err := x.startBuffer("", nil); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if c := x.write(chunk); c.kind != endNormal {
						b.Fatal(c)
					}
				}
				b.StopTimer()
				return completion{}
			})
		})
	}
}

// BenchmarkExecutionCall measures PHP calls that each register two locals,
// nested three deep, as user functions do.
func BenchmarkExecutionCall(b *testing.B) {
	r, _ := NewRequest(nil)
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		b.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		b.Fatal(err)
	}
	if err := r.Ready(); err != nil {
		b.Fatal(err)
	}
	x, err := newExecution(context.Background(), r, 64, nil, executionLimits{64, 1 << 62, 1 << 20})
	if err != nil {
		b.Fatal(err)
	}
	cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var call func(x *execution, depth int) completion
	call = func(x *execution, depth int) completion {
		return x.call(func(x *execution) completion {
			var a, c value
			if err := x.local(&a); err != nil {
				b.Fatal(err)
			}
			if err := x.local(&c); err != nil {
				b.Fatal(err)
			}
			if depth > 0 {
				return call(x, depth-1)
			}
			return completion{}
		}, nil)
	}
	x.run(cleanup, func(x *execution) completion {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if c := call(x, 2); c.kind != endNormal {
				b.Fatal(c)
			}
		}
		b.StopTimer()
		return completion{}
	})
}
