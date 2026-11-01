package runtime

import (
	"context"
	"time"
)

var testCleanupContext, _ = context.WithTimeout(context.Background(), time.Hour)

func newTestRequest() *Request {
	r, err := NewRequest(nil)
	if err != nil {
		panic(err)
	}
	return r
}
