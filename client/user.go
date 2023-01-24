package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
)

var (
	userBaseURL = "https://user.mypikpak.com"
	tokenUrl    = userBaseURL + "/v1/auth/token"
	signInUrl   = userBaseURL + "/v1/auth/signin"
)

type UserClient struct {
	*Client

	http     *http.Client
	mu       sync.Mutex
	initOnce sync.Once

	captchaSign  string
	captchaToken string
}

type userRoundTripper struct {
	*UserClient
}

func (p *userRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("authority", "user.mypikpak.com")
	req.Header.Set("accept", "*/*")
	req.Header.Set("origin", "https://mypikpak.com")
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-site")
	req.Header.Set("x-client-id", global.ClientID)
	req.Header.Set("x-client-version", global.ClientVersion)
	req.Header.Set("x-device-id", p.State.DeviceID)
	req.Header.Set("x-device-model", "chrome%2F108.0.0.0")
	req.Header.Set("x-device-name", "PC-Chrome")
	req.Header.Set("x-device-sign", "wdi10."+p.State.DeviceID+"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	req.Header.Set("x-net-work-type", "NONE")
	req.Header.Set("x-os-version", "MacIntel")
	req.Header.Set("x-platform-version", "1")
	req.Header.Set("x-protocol-version", "301")
	req.Header.Set("x-provider-name", "NONE")
	req.Header.Set("x-sdk-version", "5.2.0")

	return global.http.Transport.RoundTrip(req)
}

func (c *UserClient) init() error {
	var err error
	c.initOnce.Do(func() {
		c.http = &http.Client{}
		*c.http = *http.DefaultClient
		c.http.Transport = &userRoundTripper{c}

		c.State.DeviceID, err = c.genDeviceID()
		c.captchaSign = c.genCaptchaSign()
	})
	return err
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type signInRequest struct {
	ClientID string `json:"client_id"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type signInResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (c *UserClient) genDeviceID() (string, error) {
	u, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(u.String(), "-", ""), nil
}

func (c *UserClient) claims() (*jwt.StandardClaims, error) {
	claims := jwt.StandardClaims{}
	token, _ := jwt.ParseWithClaims(c.State.User.AccessToken, &claims, func(token *jwt.Token) (interface{}, error) {
		return nil, nil
	})
	if token == nil {
		return nil, fmt.Errorf("invalid token")
	}
	return &claims, nil
}

func (c *UserClient) Claims() (*jwt.StandardClaims, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.init()
	if err != nil {
		return nil, err
	}
	err = c.updateToken()
	if err != nil {
		return nil, err
	}
	return c.claims()
}

func (c *UserClient) signIn() error {
	reqData := signInRequest{
		ClientID: global.ClientID,
		Username: c.Config.User.Username,
		Password: c.Config.User.Password,
	}
	reqBody, err := json.Marshal(reqData)
	if err != nil {
		return err
	}
	resp, err := c.http.Post(signInUrl, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code: %d, %s", resp.StatusCode, string(body))
	}
	var respData signInResponse
	err = json.Unmarshal(body, &respData)
	if err != nil {
		return err
	}
	c.State.User.AccessToken = respData.AccessToken
	c.State.User.RefreshToken = respData.RefreshToken
	return c.SaveState()
}

func (c *UserClient) refreshToken() error {
	if c.State.User.RefreshToken == "" {
		return fmt.Errorf("refresh token is empty")
	}
	resp, err := c.http.Post(
		tokenUrl,
		"application/x-www-form-urlencoded",
		bytes.NewBufferString("grant_type=refresh_token&client_id="+global.ClientID+"&refresh_token="+c.State.User.RefreshToken),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		// clear tokens: refresh token is invalid
		c.State.User.AccessToken = ""
		c.State.User.RefreshToken = ""
		if resp.StatusCode == http.StatusUnauthorized {
			return ErrAuthorizationFailed
		}
		return fmt.Errorf("status code: %d, %s", resp.StatusCode, string(body))
	}
	var response refreshResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return err
	}
	c.State.User.AccessToken = response.AccessToken
	c.State.User.RefreshToken = response.RefreshToken

	return c.SaveState()
}

func (c *UserClient) updateToken() error {
	claims, err := c.claims()
	if err == nil {
		return nil
	}
	if err != nil && claims != nil && claims.Valid() == nil {
		return nil
	}
	err = c.refreshToken()
	if err == nil {
		return nil
	}
	return c.signIn()
}

func (c *UserClient) logout() error {
	c.State.User.AccessToken = ""
	c.State.User.RefreshToken = ""
	return c.SaveState()
}

func (c *UserClient) SignIn() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.init()
	if err != nil {
		return err
	}

	c.State.User.AccessToken = ""
	err = c.updateToken()
	if err != nil {
		return err
	}
	return c.SaveState()
}
