package bogdanfinn

import (
	"aurora/httpclient"
	"fmt"
	"io"
	"net/http"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type TlsClient struct {
	Client    tls_client.HttpClient
	ReqBefore handler
}

type handler func(r *fhttp.Request) error

func NewStdClient() *TlsClient {
	client, _ := tls_client.NewHttpClient(tls_client.NewNoopLogger(), []tls_client.HttpClientOption{
		tls_client.WithCookieJar(tls_client.NewCookieJar()),
		tls_client.WithRandomTLSExtensionOrder(),
		tls_client.WithTimeoutSeconds(600),
		tls_client.WithClientProfile(profiles.Chrome_152),
	}...)

	stdClient := &TlsClient{Client: client}
	return stdClient
}

// ChromeMajor 必须与上面的 TLS profile 同一版本; UA / sec-ch-ua 都从它推出来.
const ChromeMajor = 152

// ChromeUA 是与 TLS profile 对应的 Windows Chrome UA.
var ChromeUA = fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.0.0 Safari/537.36", ChromeMajor)

var ChromeSecChUA = secChUA(ChromeMajor)

// secChUA 按 Chromium GenerateBrandVersionList 的 GREASE 规则生成 sec-ch-ua.
func secChUA(m int) string {
	chars := []string{" ", "(", ":", "-", ".", "/", ")", ";", "=", "?", "_"}
	versions := []string{"8", "99", "24"}
	orders := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	order := orders[m%6]
	brands := make([]string, 3)
	brands[order[0]] = fmt.Sprintf(`"Not%sA%sBrand";v="%s"`, chars[m%11], chars[(m+1)%11], versions[m%3])
	brands[order[1]] = fmt.Sprintf(`"Chromium";v="%d"`, m)
	brands[order[2]] = fmt.Sprintf(`"Google Chrome";v="%d"`, m)
	return strings.Join(brands, ", ")
}

// chromeHeaderOrder 是 Chrome 发 fetch/导航请求时的头顺序; 未列出的头排在后面.
var chromeHeaderOrder = []string{
	"content-length", "cache-control", "sec-ch-ua-platform", "x-vqd-hash-1", "x-fe-version",
	"x-fe-signals", "x-ddg-journey-id", "x-vqd-accept", "user-agent", "sec-ch-ua", "content-type",
	"sec-ch-ua-mobile", "upgrade-insecure-requests", "accept", "origin", "sec-fetch-site",
	"sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest", "referer", "accept-encoding",
	"accept-language", "cookie", "priority",
}

func convertResponse(resp *fhttp.Response) *http.Response {
	response := &http.Response{
		Status:           resp.Status,
		StatusCode:       resp.StatusCode,
		Proto:            resp.Proto,
		ProtoMajor:       resp.ProtoMajor,
		ProtoMinor:       resp.ProtoMinor,
		Header:           http.Header(resp.Header),
		Body:             resp.Body,
		ContentLength:    resp.ContentLength,
		TransferEncoding: resp.TransferEncoding,
		Close:            resp.Close,
		Uncompressed:     resp.Uncompressed,
		Trailer:          http.Header(resp.Trailer),
	}
	return response
}

func (t *TlsClient) handleHeaders(req *fhttp.Request, headers httpclient.AuroraHeaders) {
	if headers == nil {
		return
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}

func (t *TlsClient) handleCookies(req *fhttp.Request, cookies []*http.Cookie) {
	if cookies == nil {
		return
	}
	for _, c := range cookies {
		req.AddCookie(&fhttp.Cookie{
			Name:       c.Name,
			Value:      c.Value,
			Path:       c.Path,
			Domain:     c.Domain,
			Expires:    c.Expires,
			RawExpires: c.RawExpires,
			MaxAge:     c.MaxAge,
			Secure:     c.Secure,
			HttpOnly:   c.HttpOnly,
			SameSite:   fhttp.SameSite(c.SameSite),
			Raw:        c.Raw,
			Unparsed:   c.Unparsed,
		})
	}
}

func (t *TlsClient) Request(method httpclient.HttpMethod, url string, headers httpclient.AuroraHeaders, cookies []*http.Cookie, body io.Reader) (*http.Response, error) {
	req, err := fhttp.NewRequest(string(method), url, body)
	if err != nil {
		return nil, err
	}
	t.handleHeaders(req, headers)
	t.handleCookies(req, cookies)
	req.Header[fhttp.HeaderOrderKey] = chromeHeaderOrder
	if t.ReqBefore != nil {
		if err := t.ReqBefore(req); err != nil {
			return nil, err
		}
	}
	do, err := t.Client.Do(req)
	if err != nil {
		return nil, err
	}
	return convertResponse(do), nil
}

func (t *TlsClient) SetProxy(url string) error {
	return t.Client.SetProxy(url)
}
