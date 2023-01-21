package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/gyf304/pikpakdav/client"
)

var (
	port = 8080
)

func init() {
	portEnv := os.Getenv("PORT")
	if portEnv != "" {
		newPort, _ := strconv.Atoi(portEnv)
		if newPort > 0 && newPort < 65536 {
			port = newPort
		}
	}
}

type authHandler struct {
	clients map[string]*client.Client
	mu      sync.Mutex
}

type userInfo struct {
	Username string
	Password string
}

func parseBasicAuth(r *http.Request) (*userInfo, error) {
	s := r.Header.Get("Authorization")
	if s == "" {
		return nil, fmt.Errorf("no authorization header")
	}
	if len(s) < 6 || s[:6] != "Basic " {
		return nil, fmt.Errorf("not basic auth")
	}
	b, err := base64.StdEncoding.DecodeString(s[6:])
	if err != nil {
		return nil, err
	}
	pair := bytes.SplitN(b, []byte(":"), 2)
	if len(pair) != 2 {
		return nil, fmt.Errorf("invalid basic auth")
	}
	return &userInfo{
		Username: string(pair[0]),
		Password: string(pair[1]),
	}, nil
}

func (a *authHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, err := parseBasicAuth(r)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="pikpakdav"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("401 Unauthorized"))
		return
	}

	a.mu.Lock()
	c := a.clients[u.Username]
	if c == nil {
		c = &client.Client{}
		c.Config.User.Username = u.Username
		c.Config.User.Password = u.Password
		uc, err := c.User()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("500 Internal Server Error"))
			return
		}
		err = uc.SignIn()
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("401 Unauthorized"))
			return
		}
		a.clients[u.Username] = c
	} else if c.Config.User.Password != u.Password {
		c.Config.User.Password = u.Password
		c.State.User.AccessToken = ""
		c.State.User.RefreshToken = ""
		uc, err := c.User()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("500 Internal Server Error"))
			return
		}
		err = uc.SignIn()
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("401 Unauthorized"))
			return
		}
		a.clients[u.Username] = c
	}
	a.mu.Unlock()

	d, err := c.Drive()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 Internal Server Error"))
		return
	}

	dav, err := d.WebDAV()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 Internal Server Error"))
		return
	}

	dav.ServeHTTP(w, r)
}

func main() {
	http.Handle("/", &authHandler{
		clients: make(map[string]*client.Client),
	})
	panic(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
