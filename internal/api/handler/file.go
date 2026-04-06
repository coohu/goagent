package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/coohu/goagent/internal/agent"
	"github.com/gin-gonic/gin"
)

type FileHandler struct {
	sessions      *agent.SessionManager
	workspaceRoot string
}

func NewFileHandler(sessions *agent.SessionManager, workspaceRoot string) *FileHandler {
	return &FileHandler{sessions: sessions, workspaceRoot: workspaceRoot}
}

func (h *FileHandler) Upload(c *gin.Context) {
	sessionID := c.Param("session_id")
	if _, err := h.sessions.Get(sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form"})
		return
	}

	destDir := h.sessionWorkspace(sessionID)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot create workspace"})
		return
	}

	var uploaded []string
	for _, files := range form.File {
		for _, fh := range files {
			name := filepath.Base(fh.Filename)
			if name == "" || name == "." {
				continue
			}
			dest := filepath.Join(destDir, name)

			src, err := fh.Open()
			if err != nil {
				continue
			}
			dst, err := os.Create(dest)
			if err != nil {
				src.Close()
				continue
			}
			io.Copy(dst, src)
			src.Close()
			dst.Close()
			uploaded = append(uploaded, name)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"uploaded":  uploaded,
		"workspace": destDir,
	})
}

func (h *FileHandler) Download(c *gin.Context) {
	sessionID := c.Param("session_id")
	if _, err := h.sessions.Get(sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	relPath := c.Query("path")
	if relPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query param required"})
		return
	}

	fullPath := filepath.Join(h.sessionWorkspace(sessionID), filepath.Clean(relPath))
	if !strings.HasPrefix(fullPath, h.workspaceRoot) {
		c.JSON(http.StatusForbidden, gin.H{"error": "path outside workspace"})
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(fullPath)))
	c.File(fullPath)
}

func (h *FileHandler) List(c *gin.Context) {
	sessionID := c.Param("session_id")
	if _, err := h.sessions.Get(sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	dir := h.sessionWorkspace(sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"files": []any{}})
		return
	}

	type fileInfo struct {
		Name  string `json:"name"`
		Size  int64  `json:"size"`
		IsDir bool   `json:"is_dir"`
	}
	files := make([]fileInfo, 0, len(entries))
	for _, e := range entries {
		info, _ := e.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		files = append(files, fileInfo{Name: e.Name(), Size: size, IsDir: e.IsDir()})
	}
	c.JSON(http.StatusOK, gin.H{"files": files, "workspace": dir})
}

func (h *FileHandler) sessionWorkspace(sessionID string) string {
	return filepath.Join(h.workspaceRoot, sessionID)
}
