package client

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type SigningAlgorithm struct {
	Alg  string `json:"alg"`
	Salt string `json:"salt"`
}

type captchaTokenCacheItem struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
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

func (c *UserClient) genCaptchaSign() string {
	s := &c.State
	signStr := global.ClientID + global.ClientVersion + global.PackageName + s.DeviceID + global.Timestamp
	for _, alg := range global.SigningAlgorithms {
		md5sum := md5.New()
		md5sum.Write([]byte(signStr + alg.Salt))
		signStr = hex.EncodeToString(md5sum.Sum(nil))
	}
	return "1." + signStr
}

func (c *UserClient) getCaptchaToken(action string, retries int) (string, error) {
	if retries < 0 {
		return "", errors.New("captcha token retries exceeded")
	}
	req := captchaInitRequest{
		ClientID:     global.ClientID,
		Action:       action,
		DeviceID:     c.State.DeviceID,
		CaptchaToken: c.captchaToken,
	}

	if strings.Contains(action, "signin") {
		req.Meta.Email = c.Config.User.Username
	} else {
		claims, err := c.claims()
		if err != nil {
			return "", err
		}
		req.Meta.CaptchaSign = c.captchaSign
		req.Meta.ClientVersion = global.ClientVersion
		req.Meta.PackageName = global.PackageName
		req.Meta.UserID = claims.Subject
		req.Meta.Timestamp = global.Timestamp
	}

	marshalled, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Post(
		userBaseURL+"/v1/shield/captcha/init",
		"application/json",
		bytes.NewReader(marshalled),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		c.State.User.AccessToken = ""
		return c.getCaptchaToken(action, retries-1)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("getCaptchaToken: %s, %s", resp.Status, body)
	}
	var captchaResp captchaInitResponse

	err = json.Unmarshal(body, &captchaResp)
	if err != nil {
		return "", err
	}

	c.captchaToken = captchaResp.CaptchaToken
	c.SaveState()

	return captchaResp.CaptchaToken, nil
}

func (c *UserClient) SignRequest(req *http.Request) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.init()
	if err != nil {
		return err
	}

	action := req.Method + ":" + req.URL.Path

	err = c.updateToken()
	if err != nil {
		return err
	}

	t, err := c.getCaptchaToken(action, 1)

	if err != nil {
		return err
	}
	req.Header.Set("x-captcha-token", t)
	req.Header.Set("Authorization", "Bearer "+c.State.User.AccessToken)

	return nil
}
