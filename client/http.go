package client

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/sync/semaphore"
)

type pikPakUserRoundTripper struct {
	c *Client
}

func (p *pikPakUserRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("authority", "user.mypikpak.com")
	req.Header.Set("accept", "*/*")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("dnt", "1")
	req.Header.Set("origin", "https://mypikpak.com")
	req.Header.Set("Referer", "https://mypikpak.com/")
	req.Header.Set("sec-ch-ua", "\"Google Chrome\";v=\"87\", \" Not;A Brand\";v=\"99\", \"Chromium\";v=\"87\"")
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", "\"macOS\"")
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-site")
	req.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36")
	req.Header.Set("x-client-id", p.c.State.ClientID)
	req.Header.Set("x-client-version", p.c.State.ClientVersion)
	req.Header.Set("x-device-id", p.c.State.DeviceID)
	req.Header.Set("x-device-model", "chrome%2F108.0.0.0")
	req.Header.Set("x-device-name", "PC-Chrome")
	req.Header.Set("x-device-sign", "wdi10."+p.c.State.DeviceID+"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	req.Header.Set("x-net-work-type", "NONE")
	req.Header.Set("x-os-version", "MacIntel")
	req.Header.Set("x-platform-version", "1")
	req.Header.Set("x-protocol-version", "301")
	req.Header.Set("x-provider-name", "NONE")
	req.Header.Set("x-sdk-version", "5.2.0")
	return http.DefaultTransport.RoundTrip(req)
}

type pikPakDriveApiRoundTripper struct {
	c *Client
}

func (p *pikPakDriveApiRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	action := req.Method + ":" + req.URL.Path
	captchaToken, err := p.c.GetCaptchaToken(action)
	if err != nil {
		return nil, err
	}
	oauth2, err := p.c.GetOAuth2()
	if err != nil {
		return nil, err
	}
	req.Header.Set("dnt", "1")
	req.Header.Set("accept-language", "en-US")
	req.Header.Set("origin", "https://mypikpak.com")
	req.Header.Set("Referer", "https://mypikpak.com/")
	req.Header.Set("sec-ch-ua", "\"Google Chrome\";v=\"87\", \" Not;A Brand\";v=\"99\", \"Chromium\";v=\"87\"")
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", "\"macOS\"")
	req.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36")
	req.Header.Set("x-device-id", p.c.State.DeviceID)
	req.Header.Set("Authorization", "Bearer "+oauth2.AccessToken)
	req.Header.Set("x-captcha-token", captchaToken)

	return http.DefaultTransport.RoundTrip(req)
}

type rcCloseWrapper struct {
	io.ReadCloser
	close func()
}

func (r *rcCloseWrapper) Close() error {
	r.close()
	return r.ReadCloser.Close()
}

type pikPakDownloadRoundTripper struct {
	c *Client
	s *semaphore.Weighted
}

func (p *pikPakDownloadRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("dnt", "1")
	req.Header.Set("accept-language", "en-US")
	req.Header.Set("Referer", "https://mypikpak.com/")
	req.Header.Set("sec-ch-ua", "\"Google Chrome\";v=\"87\", \" Not;A Brand\";v=\"99\", \"Chromium\";v=\"87\"")
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", "\"macOS\"")
	req.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36")
	req.Header.Set("x-device-id", p.c.State.DeviceID)

	ctx := req.Context()
	closer := func() {}
	if p.s != nil {
		if err := p.s.Acquire(ctx, 1); err != nil {
			return nil, err
		}
		once := sync.Once{}
		closer = func() {
			once.Do(func() {
				p.s.Release(1)
			})
		}
	}

	resp, err := http.DefaultTransport.RoundTrip(req)

	contentRange := resp.Header.Get("Content-Range")
	expectedRange := req.Header.Get("Range")
	if expectedRange != "" {
		if resp.StatusCode != http.StatusPartialContent {
			err = fmt.Errorf("expected status code %d, got %s", http.StatusPartialContent, resp.Status)
			return nil, err
		}
		expectedRange = strings.Replace(expectedRange, "=", " ", 1)
		if !strings.HasPrefix(contentRange, expectedRange) {
			err = fmt.Errorf("expected range %s, got %s", expectedRange, contentRange)
			return nil, err
		}
	}

	body := resp.Body
	resp.Body = &rcCloseWrapper{
		ReadCloser: body,
		close:      closer,
	}

	return resp, err
}
