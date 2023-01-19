package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang-jwt/jwt"
)

var (
	tokenUrl  = userBaseURL + "/v1/auth/token"
	signInUrl = userBaseURL + "/v1/auth/signin"
)

type OAuth2 struct {
	RefreshToken string `json:"refreshToken"`
	AccessToken  string `json:"accessToken"`
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

func (t *OAuth2) Claims() *jwt.StandardClaims {
	claims := jwt.StandardClaims{}
	token, _ := jwt.ParseWithClaims(t.AccessToken, &claims, func(token *jwt.Token) (interface{}, error) {
		return nil, nil
	})
	if token == nil {
		return nil
	}
	return &claims
}

func (c *Client) signIn() error {
	reqData := signInRequest{
		ClientID: c.State.ClientID,
		Username: c.Config.Username,
		Password: c.Config.Password,
	}
	reqBody, err := json.Marshal(reqData)
	if err != nil {
		return err
	}
	resp, err := c.userHTTPClient.Post(signInUrl, "application/json", bytes.NewBuffer(reqBody))
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
	c.State.OAuth2.AccessToken = respData.AccessToken
	c.State.OAuth2.RefreshToken = respData.RefreshToken
	return c.SaveState()
}

func (c *Client) refreshToken() error {
	t := &c.State.OAuth2
	resp, err := c.userHTTPClient.Post(
		tokenUrl,
		"application/x-www-form-urlencoded",
		bytes.NewBufferString("grant_type=refresh_token&client_id="+c.State.ClientID+"&refresh_token="+t.RefreshToken),
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
		t.AccessToken = ""
		t.RefreshToken = ""
		return fmt.Errorf("status code: %d, %s", resp.StatusCode, string(body))
	}
	var response refreshResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return err
	}
	t.AccessToken = response.AccessToken
	t.RefreshToken = response.RefreshToken

	return c.SaveState()
}

func (c *Client) updateToken() error {
	claims := c.State.OAuth2.Claims()
	if claims != nil && claims.Valid() == nil {
		return nil
	}
	err := c.refreshToken()
	if err == nil {
		return nil
	}
	return c.signIn()
}

func (c *Client) GetOAuth2() (*OAuth2, error) {
	err := c.updateToken()
	if err != nil {
		return nil, err
	}
	var oAuth2 OAuth2
	oAuth2 = c.State.OAuth2
	return &oAuth2, nil
}

func (c *Client) InvalidateAccessToken() {
	c.State.OAuth2.AccessToken = ""
}

func (c *Client) SignIn() error {
	c.State.OAuth2.AccessToken = ""
	err := c.updateToken()
	if err != nil {
		return err
	}
	return c.SaveState()
}
