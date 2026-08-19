package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestTaskBehavior(t *testing.T) {
	ResetTaskHTTPState()
	codes := make(chan int, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, worker := range []string{"w1", "w2"} {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rr := httptest.NewRecorder()
			TaskHTTPHandler(rr, httptest.NewRequest("POST", "/task?id=h1&worker="+worker, nil))
			codes <- rr.Code
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	ok, conflict := 0, 0
	for code := range codes {
		if code == http.StatusNoContent {
			ok++
		}
		if code == http.StatusConflict {
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("ok=%d conflict=%d", ok, conflict)
	}
}
