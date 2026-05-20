package storage

import (
	"bufio"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/pkg/storage/core"
)

func SourceHandler(fs *FileSystem, secret string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fs == nil {
			http.NotFound(w, r)
			return
		}

		query := r.URL.Query()
		objectPath := strings.TrimSpace(query.Get("path"))
		expires := query.Get("expires")
		signature := query.Get("signature")
		if objectPath == "" || expires == "" || signature == "" {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		if err := core.VerifySourceSignature(objectPath, expires, signature, secret, time.Now()); err != nil {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		result := fs.GetWithHook(r.Context(), objectPath)
		if result.Error != nil || result.Reader == nil {
			http.NotFound(w, r)
			return
		}
		defer result.Reader.Close()

		name := path.Base(objectPath)
		reader := bufio.NewReader(result.Reader)
		w.Header().Set("Content-Disposition", sourceDisposition(query.Get("download"), name))
		w.Header().Set("Content-Type", sourceContentType(name, reader))
		if _, err := io.Copy(w, reader); err != nil {
			return
		}
	})
}

func sourceDisposition(download string, name string) string {
	disposition := "inline"
	if download == "1" || strings.EqualFold(download, "true") {
		disposition = "attachment"
	}
	return mime.FormatMediaType(disposition, map[string]string{"filename": name})
}

func sourceContentType(name string, reader *bufio.Reader) string {
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		return contentType
	}
	data, err := reader.Peek(512)
	if err != nil && len(data) == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(data)
}
