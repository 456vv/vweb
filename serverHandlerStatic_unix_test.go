//go:build unix

package vweb

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestNonRegularFileForbidden 验证命名管道在 Open 之前被拒绝，不会阻塞请求。
func TestNonRegularFileForbidden(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}

	h := &ServerHandlerStatic{RootPath: dir}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/pipe", nil)

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ServeHTTP blocked on FIFO; should reject non-regular file before Open")
	}

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
