package client

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/flynn/json5"
	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"
)

const (
	baseURL         = "https://mypikpak.com"
	userBaseURL     = "https://user.mypikpak.com"
	driveAPIBaseURL = "https://api-drive.mypikpak.com"
)

var (
	mainJsRegexp        = regexp.MustCompile(`/drive/main\.[a-z0-9]+\.js`)
	clientIDRegexp      = regexp.MustCompile(`clientId:"([A-Za-z0-9]+)"`)
	clientVersionRegexp = regexp.MustCompile(`clientVersion:"([0-9.]+)"`)
	packageNameRegexp   = regexp.MustCompile(`packageName:"([a-z0-9.]+)"`)
	timestampRegexp     = regexp.MustCompile(`timestamp:"([0-9]+)"`)
	algorithmsRegexp    = regexp.MustCompile(`algorithms:(\[[^\]]+\])`)
)

func findFirstStringSubmatch(re *regexp.Regexp, s string) string {
	match := re.FindStringSubmatch(s)
	if match == nil {
		return ""
	}
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func (c *Client) Initialize() error {
	resp1, err := http.Get(baseURL + "/drive")
	if err != nil {
		return err
	}

	defer resp1.Body.Close()
	body1, err := ioutil.ReadAll(resp1.Body)
	if err != nil {
		return err
	}
	mainJs := string(mainJsRegexp.Find(body1))
	if mainJs == "" {
		return fmt.Errorf("main.js not found")
	}

	resp2, err := http.Get(baseURL + mainJs)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	body2, err := ioutil.ReadAll(resp2.Body)
	if err != nil {
		return err
	}

	c.State.ClientID = findFirstStringSubmatch(clientIDRegexp, string(body2))
	c.State.ClientVersion = findFirstStringSubmatch(clientVersionRegexp, string(body2))
	c.State.PackageName = findFirstStringSubmatch(packageNameRegexp, string(body2))
	c.State.Timestamp = findFirstStringSubmatch(timestampRegexp, string(body2))

	algorithms := findFirstStringSubmatch(algorithmsRegexp, string(body2))
	err = json5.Unmarshal([]byte(algorithms), &c.State.Signing.Algorithms)
	if err != nil {
		return err
	}

	if c.State.DeviceID == "" {
		c.State.DeviceID = strings.ReplaceAll(uuid.New().String(), "-", "")
	}

	c.State.Signing.CaptchaSign = c.genCaptchaSign()
	c.userHTTPClient = &http.Client{}
	*c.userHTTPClient = *http.DefaultClient
	c.userHTTPClient.Transport = &pikPakUserRoundTripper{c}

	c.driveApiHTTPClient = &http.Client{}
	*c.driveApiHTTPClient = *http.DefaultClient
	c.driveApiHTTPClient.Transport = &pikPakDriveApiRoundTripper{c}

	c.downloadHTTPClient = &http.Client{}
	*c.downloadHTTPClient = *http.DefaultClient
	c.downloadHTTPClient.Transport = &pikPakDownloadRoundTripper{
		c: c,
		s: semaphore.NewWeighted(2),
	}

	c.SaveState()

	return nil
}
