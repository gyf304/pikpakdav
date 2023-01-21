package client

import (
	"net/http"
	"sync"
)

type Client struct {
	State  State
	Config Config

	StateFile  string
	ConfigFile string

	user       UserClient
	drive      DriveClient
	davHandler *http.Handler

	initOnce sync.Once
}

func (c *Client) init() error {
	c.initOnce.Do(func() {
		c.user.Client = c
		c.drive.Client = c
	})

	return nil
}

func (c *Client) User() (*UserClient, error) {
	err := c.init()
	if err != nil {
		return nil, err
	}

	return &c.user, nil
}

func (c *Client) Drive() (*DriveClient, error) {
	err := c.init()
	if err != nil {
		return nil, err
	}

	return &c.drive, nil
}
