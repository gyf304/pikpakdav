package client

import (
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/webdav"
	"golang.org/x/sync/semaphore"
)

var (
	maxDownloadConnections = 2
)

type webdavHandler struct {
	h   *webdav.Handler
	fs  *FileSystem
	sem *semaphore.Weighted
}

func (h *webdavHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// directly serve the file, bypassing webdav
		ctx := r.Context()
		err := h.sem.Acquire(ctx, 1)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer h.sem.Release(1)

		item, err := h.fs.getDriveItem(ctx, r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if item.IsFolder() {
			http.Error(w, "not a file", http.StatusNotFound)
			return
		}
		f, err := h.fs.cachedFetch(ctx, item)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		link := f.WebContentLink
		req2 := r.Clone(ctx)
		req2.URL, err = url.Parse(link)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req2.Header.Del("Host")
		req2.RequestURI = ""
		h := global.http
		resp, err := h.Do(req2)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		code := resp.StatusCode
		if code == http.StatusServiceUnavailable {
			code = http.StatusTooManyRequests
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Retry-After", "10")
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(code)
			// most clients will retry immediately, so we hold the request for a second
			time.Sleep(1 * time.Second)
			_, err = w.Write(body)
			return
		}
		w.WriteHeader(code)
		_, err = io.Copy(w, resp.Body)
		return
	}
	h.h.ServeHTTP(w, r)
}

func (c *DriveClient) NewWebDAVHandler() (http.Handler, error) {
	fs, err := c.FileSystem()
	if err != nil {
		return nil, err
	}
	davHandler := &webdav.Handler{
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
	}
	return &webdavHandler{
		h:   davHandler,
		fs:  fs,
		sem: semaphore.NewWeighted(int64(maxDownloadConnections)),
	}, nil
}
