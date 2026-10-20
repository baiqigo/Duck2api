package duckgo

import (
	"aurora/httpclient"
	"aurora/httpclient/bogdanfinn"
	duckgotypes "aurora/typings/duckgo"
	officialtypes "aurora/typings/official"
	"aurora/util"
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	FEVersion = &XqdgToken{}
	UA        = bogdanfinn.ChromeUA
	// bundleStack 是首页当前 entry.duckai.*.js 拼出的 meta.stack, 随 warmSession 刷新.
	bundleStack   string
	bundleStackMu sync.RWMutex
	bundleRe      = regexp.MustCompile(`entry\.duckai\.[0-9a-f]+\.js`)
)

type XqdgToken struct {
	Token    string     `json:"token"`
	M        sync.Mutex `json:"-"`
	ExpireAt time.Time  `json:"expire"`
}

// InitXVQD 按浏览器顺序走一遍: 打开首页 (拿 cookie/前端版本) -> status 取挑战 -> 本地解.
// 每次都重新算, 不跨代理/会话复用 token —— 复用或跳过首页会被 duck.ai 直接 418.
func InitXVQD(client httpclient.AuroraHttpClient, proxyUrl string) (string, error) {
	if proxyUrl != "" {
		client.SetProxy(proxyUrl)
	}
	if err := warmSession(client); err != nil {
		return "", err
	}
	status, err := postStatus(client, "")
	if err != nil {
		return "", err
	}
	defer status.Body.Close()
	vqdHash := status.Header.Get("x-vqd-hash-1")
	if vqdHash == "" {
		return "", fmt.Errorf("no x-vqd-hash-1 token (status %d)", status.StatusCode)
	}
	return GenerateVQDHash(vqdHash)
}

func postStatus(client httpclient.AuroraHttpClient, proxyUrl string) (*http.Response, error) {
	if proxyUrl != "" {
		client.SetProxy(proxyUrl)
	}
	header := createHeader()
	delete(header, "content-type")
	header.Set("accept", "*/*")
	header.Set("cache-control", "no-store")
	header.Set("x-vqd-accept", "1")
	response, err := client.Request(httpclient.GET, "https://duck.ai/duckchat/v1/status", header, nil, nil)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// POSTconversation 在当前出口发一次; 不原地重取 token (实测重取即 418), 418/429 交给调用方换出口.
func POSTconversation(client httpclient.AuroraHttpClient, request duckgotypes.ApiRequest, token string, proxyUrl string) (*http.Response, error) {
	if proxyUrl != "" {
		client.SetProxy(proxyUrl)
	}
	return postConversationOnce(client, request, token)
}

func Handle_request_error(c *gin.Context, response *http.Response) bool {
	if response.StatusCode != 200 {
		// 先整体读出再解析, 否则 decode 失败后 body 已被读空
		body, _ := io.ReadAll(response.Body)
		var error_response map[string]interface{}
		if err := json.Unmarshal(body, &error_response); err != nil {
			c.JSON(response.StatusCode, gin.H{"error": gin.H{
				"message": "Unknown error",
				"type":    "internal_server_error",
				"param":   nil,
				"code":    "500",
				"details": string(body),
			}})
			return true
		}
		c.JSON(response.StatusCode, gin.H{"error": gin.H{
			"message": error_response["detail"],
			"type":    response.Status,
			"param":   nil,
			"code":    "error",
		}})
		return true
	}
	return false
}

func createHeader() httpclient.AuroraHeaders {
	header := make(httpclient.AuroraHeaders)
	// 与 vqd.go 里 navigator.language 保持一致
	header.Set("accept-language", "en-US,en;q=0.9")
	header.Set("content-type", "application/json")
	header.Set("origin", "https://duck.ai")
	header.Set("referer", "https://duck.ai/")
	header.Set("sec-ch-ua", bogdanfinn.ChromeSecChUA)
	header.Set("sec-ch-ua-mobile", "?0")
	header.Set("sec-ch-ua-platform", `"Windows"`)
	header.Set("sec-fetch-dest", "empty")
	header.Set("sec-fetch-mode", "cors")
	header.Set("sec-fetch-site", "same-origin")
	header.Set("user-agent", UA)
	return header
}

func postConversationOnce(client httpclient.AuroraHttpClient, request duckgotypes.ApiRequest, token string) (*http.Response, error) {
	bodyJSON, err := json.Marshal(request)
	if err != nil {
		return &http.Response{}, err
	}
	header := createHeader()
	header.Set("accept", "text/event-stream")
	header.Set("priority", "u=1, i")
	header.Set("x-ddg-journey-id", RandomHex(16))
	header.Set("x-fe-signals", CreateFESignals())
	if feVersion, err := InitFEVersion(client, ""); err == nil && feVersion != "" {
		header.Set("x-fe-version", feVersion)
	}
	header.Set("x-vqd-hash-1", token)
	return client.Request(httpclient.POST, "https://duck.ai/duckchat/v1/chat", header, nil, bytes.NewBuffer(bodyJSON))
}

func InitFEVersion(client httpclient.AuroraHttpClient, proxyUrl string) (string, error) {
	fe := feVersionCache()
	fe.M.Lock()
	if fe.Token != "" && fe.ExpireAt.After(time.Now()) {
		defer fe.M.Unlock()
		return fe.Token, nil
	}
	fe.M.Unlock()

	if proxyUrl != "" {
		client.SetProxy(proxyUrl)
	}
	if err := warmSession(client); err != nil {
		return "", err
	}
	fe.M.Lock()
	defer fe.M.Unlock()
	return fe.Token, nil
}

func feVersionCache() *XqdgToken {
	if FEVersion == nil { // 测试里可能手动置空
		FEVersion = &XqdgToken{}
	}
	return FEVersion
}

// warmSession 以浏览器导航的方式打开 duck.ai 首页: 让 cookie jar 拿到会话 cookie,
// 顺便刷新 x-fe-version 缓存.
func warmSession(client httpclient.AuroraHttpClient) error {
	header := httpclient.AuroraHeaders{
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"accept-language":           "en-US,en;q=0.9",
		"priority":                  "u=0, i",
		"sec-ch-ua":                 bogdanfinn.ChromeSecChUA,
		"sec-ch-ua-mobile":          "?0",
		"sec-ch-ua-platform":        `"Windows"`,
		"sec-fetch-dest":            "document",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-site":            "none",
		"sec-fetch-user":            "?1",
		"upgrade-insecure-requests": "1",
		"user-agent":                UA,
	}
	response, err := client.Request(httpclient.GET, "https://duck.ai/", header, nil, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	versionTagMatch := regexp.MustCompile(`data-version-tag="([^"]+)"`).FindSubmatch(body)
	versionShaMatch := regexp.MustCompile(`data-version-sha="([^"]+)"`).FindSubmatch(body)
	if len(versionTagMatch) < 2 || len(versionShaMatch) < 2 {
		return fmt.Errorf("duck.ai version metadata not found (status %d)", response.StatusCode)
	}

	fe := feVersionCache()
	fe.M.Lock()
	defer fe.M.Unlock()
	fe.Token = fmt.Sprintf("%s-%s", versionTagMatch[1], versionShaMatch[1])
	fe.ExpireAt = time.Now().Add(30 * time.Minute)
	if bundle := bundleRe.Find(body); bundle != nil {
		bundleStackMu.Lock()
		bundleStack = fmt.Sprintf("Error\nat l (https://duck.ai/dist/duckai-dist/%s:2:1446307)\nat async https://duck.ai/dist/duckai-dist/%s:2:1294181", bundle, bundle)
		bundleStackMu.Unlock()
	}
	return nil
}

func CreateFESignals() string {
	now := time.Now().UnixMilli()
	// Reproduce the event log the duck.ai frontend records between page load
	// request): onboarding_impression -> action -> onboarding_finish -> startNewChat_free.
	// 模拟真实用户行为: 页面加载 -> 用户思考输入 -> 完成输入 -> 点击发送
	// 时间间隔调整为更接近真实用户的行为模式
	// 总时长要落在 warmSession 打开首页到发消息的真实间隔 (约 1-4 秒) 内,
	// 并与 vqd.go 里 pageStart__ (1.5-4.5 秒前) 对得上; 声称停留几十秒会与首页请求时间矛盾.
	impression := 50 + randInt63n(100)            // 50-150ms (页面加载完成)
	action := impression + 600 + randInt63n(1400) // 0.6-2秒 (粘贴/输入)
	finish := action + 200 + randInt63n(800)      // 0.2-1秒 (输入完成)
	startChat := finish + 10 + randInt63n(90)     // 10-100ms (点击发送按钮)
	end := startChat + randInt63n(10)
	payload := map[string]interface{}{
		"start": now - end,
		"events": []map[string]interface{}{
			{"name": "onboarding_impression", "delta": impression},
			{"name": "action", "delta": action, "trusted": true},
			{"name": "onboarding_finish", "delta": finish},
			{"name": "startNewChat_free", "delta": startChat},
		},
		"end": end,
	}
	body, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(body)
}

// randInt63n returns a uniform random non-negative int64 in [0, n).
func randInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return 0
	}
	var v int64
	for _, b := range buf {
		v = v<<8 | int64(b)
	}
	if v < 0 {
		v = -v
	}
	return v % n
}

func RandomHex(byteLength int) string {
	buffer := make([]byte, byteLength)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

// ResetXVQD 保留给旧调用方; token 已不再缓存, 每次 InitXVQD 都会重新取.
func ResetXVQD() {}

func ReadResponseError(response *http.Response) error {
	var errorResponse map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&errorResponse); err == nil {
		if detail, ok := errorResponse["detail"]; ok {
			return fmt.Errorf("%s: %v", response.Status, detail)
		}
		return fmt.Errorf("%s: %v", response.Status, errorResponse)
	}

	body, _ := io.ReadAll(response.Body)
	if len(body) == 0 {
		return fmt.Errorf("%s", response.Status)
	}
	return fmt.Errorf("%s: %s", response.Status, string(body))
}

func ReadResponseText(response *http.Response) string {
	reader := bufio.NewReader(response.Body)
	var previousText strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return ""
		}
		if len(line) < 6 {
			continue
		}
		line = line[6:]
		if strings.HasPrefix(line, "[DONE]") {
			continue
		}

		var originalResponse duckgotypes.ApiResponse
		err = json.Unmarshal([]byte(line), &originalResponse)
		if err != nil || originalResponse.Action != "success" {
			continue
		}
		previousText.WriteString(originalResponse.Message)
	}
	return previousText.String()
}

// HandlerStats carries pre-computed input stats and timing start into the stream handler.
type HandlerStats struct {
	Start        time.Time
	PromptTokens int
	CachedTokens int
	Effort       string
}

// StreamResult is what Handler returns: the full text plus output-side telemetry.
type StreamResult struct {
	Text         string
	OutputTokens int
	TTFTMs       int64
	TotalMs      int64
}

func Handler(c *gin.Context, response *http.Response, oldRequest duckgotypes.ApiRequest, stream bool, stats HandlerStats) StreamResult {
	reader := bufio.NewReader(response.Body)
	if stream {
		// Response content type is text/event-stream
		c.Header("Content-Type", "text/event-stream")
	} else {
		// Response content type is application/json
		c.Header("Content-Type", "application/json")
	}

	var previousText strings.Builder
	var firstTokenSet bool
	var ttftMs int64
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return StreamResult{}
		}
		if len(line) < 6 {
			continue
		}
		line = line[6:]
		if !strings.HasPrefix(line, "[DONE]") {
			var originalResponse duckgotypes.ApiResponse
			err = json.Unmarshal([]byte(line), &originalResponse)
			if err != nil {
				continue
			}
			if originalResponse.Action != "success" {
				// 流已开始输出时响应头已发出, 再写 JSON 只会混进 SSE 流
				if !(stream && firstTokenSet) {
					c.JSON(500, gin.H{"error": "Error"})
				}
				return StreamResult{}
			}
			responseString := ""
			if originalResponse.Message != "" {
				previousText.WriteString(originalResponse.Message)
				if !firstTokenSet {
					firstTokenSet = true
					ttftMs = time.Since(stats.Start).Milliseconds()
				}
				translatedResponse := officialtypes.NewChatCompletionChunkWithModel(originalResponse.Message, originalResponse.Model)
				responseString = "data: " + translatedResponse.String() + "\n\n"
			}

			if responseString == "" {
				continue
			}

			if stream {
				_, err = c.Writer.WriteString(responseString)
				if err != nil {
					return StreamResult{}
				}
				c.Writer.Flush()
			}
		} else {
			if stream {
				final_line := officialtypes.StopChunkWithModel("stop", oldRequest.Model)
				c.Writer.WriteString("data: " + final_line.String() + "\n\n")
			}
		}
	}

	fullText := previousText.String()
	outputTokens := util.CountToken(fullText)
	totalMs := time.Since(stats.Start).Milliseconds()

	// Emit the final usage + timing chunk after [DONE] in stream mode (OpenAI include_usage compatible).
	if stream {
		usageChunk := officialtypes.UsageChunk(
			oldRequest.Model,
			stats.PromptTokens,
			outputTokens,
			stats.CachedTokens,
			ttftMs,
			totalMs,
			stats.Effort,
		)
		c.Writer.WriteString("data: " + usageChunk.String() + "\n\n")
		c.Writer.Flush()
	}

	return StreamResult{
		Text:         fullText,
		OutputTokens: outputTokens,
		TTFTMs:       ttftMs,
		TotalMs:      totalMs,
	}
}
