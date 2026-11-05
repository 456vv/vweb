package vweb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerHandlerStaticRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	h := &ServerHandlerStatic{RootPath: root, PagePath: "link.txt"}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/link.txt", nil)

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden && rr.Code != http.StatusNotFound {
		t.Fatalf("expected forbidden/not found, got %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret-content") {
		t.Fatalf("secret content leaked: %s", rr.Body.String())
	}
}

func TestServerHandlerDynamicRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.go")
	if err := os.WriteFile(secret, []byte("//yaegi\npackage main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(root, "secret.go")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	h := &ServerHandlerDynamic{
		RootPath: root,
		PagePath: "secret.go",
		Module: func(name string) (DynamicTemplater, error) {
			t.Fatalf("module should not be called for escaped path, got %q", name)
			return nil, nil
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/secret.go", nil)

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected internal server error, got %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret-content") {
		t.Fatalf("secret content leaked: %s", rr.Body.String())
	}
}

func TestDotSaveStaticRejectsSymlinkDirEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	linkDir := filepath.Join(root, "out")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	d := &Dot{
		Site: &Site{
			RootDir: func(string) string { return root },
		},
	}

	if _, err := d.SaveStatic("out/x.txt"); err == nil {
		t.Fatal("expected error for symlink dir escape")
	}
}
