package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
)

const (
	driveAPIBaseURL = "https://api-drive.mypikpak.com"

	listPrefix = driveAPIBaseURL + "/drive/v1/files?thumbnail_size=SIZE_MEDIUM&limit=1000&parent_id="
	listSuffix = "&with_audit=true&filters=%7B%22trashed%22%3A%7B%22eq%22%3Afalse%7D%2C%22phase%22%3A%7B%22eq%22%3A%22PHASE_TYPE_COMPLETE%22%7D%7D"

	fetchPrefix = driveAPIBaseURL + "/drive/v1/files/"
	fetchSuffix = "?usage=FETCH"

	trashURL = driveAPIBaseURL + "/drive/v1/files:batchTrash"
)

type DriveClient struct {
	*Client

	http *http.Client

	mu       sync.Mutex
	initOnce sync.Once
}

type driveRoundTripper struct {
	*DriveClient
}

func (p *driveRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	user, err := p.Client.User()
	if err != nil {
		return nil, err
	}

	err = user.SignRequest(req)
	if err != nil {
		return nil, err
	}

	req.Header.Set("origin", "https://mypikpak.com")
	req.Header.Set("x-device-id", p.State.DeviceID)

	return global.http.Transport.RoundTrip(req)
}

func (c *DriveClient) init() error {
	c.initOnce.Do(func() {
		c.http = &http.Client{}
		*c.http = *http.DefaultClient
		c.http.Transport = &driveRoundTripper{c}
	})
	return nil
}

type DriveFileList struct {
	c     *DriveClient
	Kind  string       `json:"kind"`
	Files []*DriveItem `json:"files"`
}

func (l *DriveFileList) Get(name string) *DriveItem {
	for _, f := range l.Files {
		if f.Name == name {
			f.c = l.c
			return f
		}
	}
	return nil
}

type DriveItem struct {
	c            *DriveClient
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	ParentID     string `json:"parent_id"`
	Name         string `json:"name"`
	Size         string `json:"size"`
	CreatedTime  string `json:"created_time"`
	ModifiedTime string `json:"modified_time"`
}

type DriveFile struct {
	DriveFileList
	WebContentLink string `json:"web_content_link"`
}

func (f *DriveItem) IsFolder() bool {
	return strings.Contains(f.Kind, "folder") || strings.Contains(f.Kind, "fileList")
}

func (f *DriveItem) IsFile() bool {
	return strings.Contains(f.Kind, "file")
}

func (f *DriveItem) List(ctx context.Context) (*DriveFileList, error) {
	if !f.IsFolder() {
		return nil, errors.New("not a folder")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", listPrefix+f.ID+listSuffix, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, errors.New(string(body))
	}
	var list DriveFileList
	err = json.Unmarshal(body, &list)
	if err != nil {
		return nil, err
	}
	list.c = f.c
	return &list, nil
}

func (f *DriveItem) Trash(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", trashURL, strings.NewReader(fmt.Sprintf(`{"ids":["%s"]}`, f.ID)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return errors.New(string(body))
	}
	return nil
}

func (f *DriveItem) Fetch(ctx context.Context) (*DriveFile, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fetchPrefix+f.ID+fetchSuffix, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, errors.New(string(body))
	}
	var file DriveFile
	err = json.Unmarshal(body, &file)
	if err != nil {
		return nil, err
	}
	return &file, nil
}

func (c *DriveClient) root() (*DriveItem, error) {
	return &DriveItem{
		c:    c,
		Kind: "drive#folder",
	}, nil
}

func (c *DriveClient) Root() (*DriveItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.init()

	if err != nil {
		return nil, err
	}

	return c.root()
}
