package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/coohu/goagent/internal/agent"
	"github.com/coohu/goagent/internal/core"
	"github.com/gin-gonic/gin"
)

func newTestFileHandler(t *testing.T) (*FileHandler, *agent.SessionManager, string) {
	t.Helper()
	tmp := t.TempDir()
	mgr := agent.NewSessionManager(5)
	return NewFileHandler(mgr, tmp), mgr, tmp
}

func setupGin() {
	gin.SetMode(gin.TestMode)
}

func TestFileUpload(t *testing.T) {
	setupGin()
	h, mgr, _ := newTestFileHandler(t)

	sess, _ := mgr.Create("test goal", core.DefaultConfig())

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("files", "hello.txt")
	fw.Write([]byte("hello content"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/"+sess.ID+"/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()

	r := gin.New()
	r.POST("/api/v1/files/:session_id/upload", h.Upload)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	uploaded, _ := resp["uploaded"].([]any)
	if len(uploaded) == 0 {
		t.Error("expected at least one uploaded file")
	}
}

func TestFileDownload(t *testing.T) {
	setupGin()
	h, mgr, tmp := newTestFileHandler(t)

	sess, _ := mgr.Create("test", core.DefaultConfig())
	sessDir := filepath.Join(tmp, sess.ID)
	os.MkdirAll(sessDir, 0755)
	os.WriteFile(filepath.Join(sessDir, "output.txt"), []byte("result data"), 0644)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+sess.ID+"/download?path=output.txt", nil)
	w := httptest.NewRecorder()

	r := gin.New()
	r.GET("/api/v1/files/:session_id/download", h.Download)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "result data" {
		t.Errorf("unexpected body: %q", w.Body.String())
	}
}

func TestFileDownloadPathTraversal(t *testing.T) {
	setupGin()
	h, mgr, _ := newTestFileHandler(t)

	sess, _ := mgr.Create("test", core.DefaultConfig())

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/files/"+sess.ID+"/download?path=../../etc/passwd", nil)
	w := httptest.NewRecorder()

	r := gin.New()
	r.GET("/api/v1/files/:session_id/download", h.Download)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
		t.Errorf("path traversal should be blocked, got %d", w.Code)
	}
}

func TestFileList(t *testing.T) {
	setupGin()
	h, mgr, tmp := newTestFileHandler(t)

	sess, _ := mgr.Create("test", core.DefaultConfig())
	sessDir := filepath.Join(tmp, sess.ID)
	os.MkdirAll(sessDir, 0755)
	os.WriteFile(filepath.Join(sessDir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(sessDir, "b.txt"), []byte("b"), 0644)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+sess.ID+"/list", nil)
	w := httptest.NewRecorder()

	r := gin.New()
	r.GET("/api/v1/files/:session_id/list", h.List)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	files, _ := resp["files"].([]any)
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}
