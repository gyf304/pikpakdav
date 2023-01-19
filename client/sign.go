package client

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
)

type SigningAlgorithm struct {
	Alg  string `json:"alg"`
	Salt string `json:"salt"`
}

type captchaTokenCacheItem struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
}

type Signing struct {
	Algorithms []SigningAlgorithm `json:"algorithms"`
	// CaptchaSign is a misnomer, it's a deterministic hash of a combination of things
	// including client ID, device ID, and user ID
	CaptchaSign      string `json:"captchaSign"`
	LastCaptchaToken string `json:"lastCaptchaToken"`
	mutex            sync.Mutex
}

type captchaInitRequest struct {
	ClientID     string `json:"client_id"`
	Action       string `json:"action"`
	DeviceID     string `json:"device_id"`
	CaptchaToken string `json:"captcha_token,omitempty"`
	Meta         struct {
		Email         string `json:"email,omitempty"`
		CaptchaSign   string `json:"captcha_sign,omitempty"`
		ClientVersion string `json:"client_version,omitempty"`
		PackageName   string `json:"package_name,omitempty"`
		UserID        string `json:"user_id,omitempty"`
		Timestamp     string `json:"timestamp,omitempty"`
	} `json:"meta"`
}

type captchaInitResponse struct {
	CaptchaToken string `json:"captcha_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (c *Client) genCaptchaSign() string {
	s := &c.State
	signStr := s.ClientID + s.ClientVersion + s.PackageName + s.DeviceID + s.Timestamp
	for _, alg := range s.Signing.Algorithms {
		md5sum := md5.New()
		md5sum.Write([]byte(signStr + alg.Salt))
		signStr = hex.EncodeToString(md5sum.Sum(nil))
	}
	return "1." + signStr
}

func (c *Client) getCaptchaToken(action string, retries int) (string, error) {
	if retries < 0 {
		return "", errors.New("captcha token retries exceeded")
	}
	s := &c.State
	s.Signing.mutex.Lock()
	req := captchaInitRequest{
		ClientID:     s.ClientID,
		Action:       action,
		DeviceID:     s.DeviceID,
		CaptchaToken: s.Signing.LastCaptchaToken,
	}
	s.Signing.mutex.Unlock()

	if strings.Contains(action, "signin") {
		req.Meta.Email = c.Config.Username
	} else {
		oauth2, err := c.GetOAuth2()
		if err != nil {
			return "", err
		}
		req.Meta.CaptchaSign = s.Signing.CaptchaSign
		req.Meta.ClientVersion = s.ClientVersion
		req.Meta.PackageName = s.PackageName
		req.Meta.UserID = oauth2.Claims().Subject
		req.Meta.Timestamp = s.Timestamp
	}

	marshalled, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	resp, err := c.userHTTPClient.Post(
		userBaseURL+"/v1/shield/captcha/init",
		"application/json",
		bytes.NewReader(marshalled),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		c.InvalidateAccessToken()
		return c.getCaptchaToken(action, retries-1)
	}
	var captchaResp captchaInitResponse

	err = json.NewDecoder(resp.Body).Decode(&captchaResp)
	if err != nil {
		return "", err
	}

	s.Signing.mutex.Lock()
	s.Signing.LastCaptchaToken = captchaResp.CaptchaToken
	s.Signing.mutex.Unlock()

	c.SaveState()

	return captchaResp.CaptchaToken, nil
}

func (c *Client) GetCaptchaToken(action string) (string, error) {
	return c.getCaptchaToken(action, 1)
}
