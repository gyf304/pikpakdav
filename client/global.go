package client

import (
	"io/ioutil"
	"net/http"
	"regexp"

	"github.com/flynn/json5"
	"github.com/rs/zerolog/log"
)

var (
	baseURL = "https://mypikpak.com"

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

type globalEnv struct {
	ClientID      string `json:"clientId"`
	ClientVersion string `json:"clientVersion"`
	PackageName   string `json:"packageName"`
	Timestamp     string `json:"timestamp"`

	SigningAlgorithms []SigningAlgorithm `json:"signingAlgorithms"`
	http              *http.Client
}

var global globalEnv

type globalRoundTripper struct{}

func (g *globalRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("dnt", "1")
	req.Header.Set("Referer", "https://mypikpak.com/")
	req.Header.Set("sec-ch-ua", "\"Google Chrome\";v=\"87\", \" Not;A Brand\";v=\"99\", \"Chromium\";v=\"87\"")
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", "\"macOS\"")
	req.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36")

	partialLog := log.Debug().Str("method", req.Method).Str("url", req.URL.String())

	resp, err := http.DefaultTransport.RoundTrip(req)

	if err != nil {
		partialLog.Err(err).Msg("http request error")
	} else {
		partialLog.Str("status", resp.Status).Msg("http request")
	}

	return resp, err
}

func init() {
	global.http = &http.Client{}
	*global.http = *http.DefaultClient
	global.http.Transport = &globalRoundTripper{}

	resp1, err := http.Get(baseURL + "/drive")
	if err != nil {
		panic(err)
	}

	defer resp1.Body.Close()
	body1, err := ioutil.ReadAll(resp1.Body)
	if err != nil {
		panic(err)
	}
	mainJs := string(mainJsRegexp.Find(body1))
	if mainJs == "" {
		panic("could not find main js file")
	}

	resp2, err := http.Get(baseURL + mainJs)
	if err != nil {
		panic(err)
	}
	defer resp2.Body.Close()
	body2, err := ioutil.ReadAll(resp2.Body)
	if err != nil {
		panic(err)
	}

	global.ClientID = findFirstStringSubmatch(clientIDRegexp, string(body2))
	global.ClientVersion = findFirstStringSubmatch(clientVersionRegexp, string(body2))
	global.PackageName = findFirstStringSubmatch(packageNameRegexp, string(body2))
	global.Timestamp = findFirstStringSubmatch(timestampRegexp, string(body2))

	algorithms := findFirstStringSubmatch(algorithmsRegexp, string(body2))
	err = json5.Unmarshal([]byte(algorithms), &global.SigningAlgorithms)
	if err != nil {
		panic(err)
	}
}
