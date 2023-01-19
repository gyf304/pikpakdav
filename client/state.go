package client

import (
	"encoding/json"
	"os"
)

func (c *Client) SaveState() error {
	c.State.mutex.Lock()
	defer c.State.mutex.Unlock()

	if c.StateFile == "" {
		return nil
	}
	marshalledState, err := json.MarshalIndent(&c.State, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(c.StateFile, marshalledState, 0644)
}

func (c *Client) LoadState() error {
	c.State.mutex.Lock()
	defer c.State.mutex.Unlock()

	if c.StateFile == "" {
		return nil
	}
	stateData, err := os.ReadFile(c.StateFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(stateData, &c.State)
}
