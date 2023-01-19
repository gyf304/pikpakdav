package client

import (
	"encoding/json"
	"os"
	"sync"
)

type ClientConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
	mutex    sync.Mutex
}

func (c *Client) LoadConfig() error {
	c.Config.mutex.Lock()
	defer c.Config.mutex.Unlock()

	if c.ConfigFile == "" {
		return nil
	}
	configData, err := os.ReadFile(c.ConfigFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(configData, &c.Config)
}

func (c *Client) SaveConfig() error {
	c.Config.mutex.Lock()
	defer c.Config.mutex.Unlock()

	if c.ConfigFile == "" {
		return nil
	}
	marshalledConfig, err := json.MarshalIndent(&c.Config, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(c.ConfigFile, marshalledConfig, 0644)
}
