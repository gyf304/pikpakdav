package client

import (
	"net/http"
	"sync"
)

type ClientState struct {
	ClientID      string `json:"clientId"`
	ClientVersion string `json:"clientVersion"`
	PackageName   string `json:"packageName"`
	Timestamp     string `json:"timestamp"`

	DeviceID string `json:"deviceId"`

	OAuth2  OAuth2  `json:"oauth2"`
	Signing Signing `json:"signing"`

	mutex sync.Mutex
}

type Client struct {
	State  ClientState
	Config ClientConfig

	StateFile  string
	ConfigFile string

	userHTTPClient     *http.Client
	driveApiHTTPClient *http.Client
	downloadHTTPClient *http.Client
}
