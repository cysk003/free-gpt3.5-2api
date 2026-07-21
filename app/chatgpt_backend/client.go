package chatgpt_backend

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"chat2api/app/browserfp"
	"chat2api/app/common"
	"chat2api/app/conf"
	"chat2api/app/constant"
	"chat2api/app/so"
	"chat2api/app/token_pool"

	"github.com/aurorax-neo/tls_client_httpi"
	"github.com/aurorax-neo/tls_client_httpi/tls_client"
)

type Client struct {
	HTTP           tls_client_httpi.TCHI
	Auth           *chatRequirements
	AccAuth        string
	BaseURL        string
	ChatURL        string
	UserAgent      string
	SessionID      string
	DeviceID       string
	ConversationID string
	TeamUserID     string
	PUID           string
	Cookies        tls_client_httpi.Cookies
	Pow            Resources
	StartTime      time.Time

	// SO (session observer) state, aligned with aurora TurnStile.
	soSession    *so.Session
	soSnapshotDX string
	soChatToken  string
	soFlow       string
	soOnce       sync.Once
	soResult     string
	soErr        error
	SOToken      string
}

type chatRequirements struct {
	Persona          string    `json:"persona,omitempty"`
	Token            string    `json:"token"`
	PrepareToken     string    `json:"prepare_token,omitempty"`
	Arkose           challenge `json:"arkose"`
	Turnstile        challenge `json:"turnstile"`
	TurnstileToken   string    `json:"-"`
	ProofWork        ProofWork `json:"proofofwork"`
	So               soSegment `json:"so"`
	ForceLogin       bool      `json:"force_login"`
	SentinelReqToken string    `json:"-"`
	SentinelReqFlow  string    `json:"-"`
}

type challenge struct {
	Required bool   `json:"required"`
	Dx       string `json:"dx"`
}

// soSegment 对齐 aurora sentinel so 段。
type soSegment struct {
	Required    bool   `json:"required"`
	CollectorDX string `json:"collector_dx,omitempty"`
	SnapshotDX  string `json:"snapshot_dx,omitempty"`
}

func New(token string, retry int) (*Client, error) {
	token = strings.TrimSpace(token)
	localToken := strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	appConf := conf.GetApp()
	if accessToken, ok := appConf.DirectAccessToken(localToken); ok {
		return newClient("Bearer "+accessToken, "")
	}
	if strings.HasPrefix(token, "Bearer eyJhbGciOiJSUzI1NiI") {
		return newClient(token, "")
	}
	if !token_pool.GetAccessTokenPool().IsEmpty() {
		accessToken := token_pool.GetAccessTokenPool().GetAccessToken()
		if accessToken == nil || accessToken.Token == "" {
			return nil, fmt.Errorf("access token pool is empty")
		}
		client, err := newClient(accessToken.Token, accessToken.Proxy)
		if client == nil && retry > 0 {
			return New(token, retry-1)
		}
		if client != nil {
			applyAccountMeta(client, accessToken)
		}
		return client, err
	}
	if strings.HasPrefix(localToken, "sk-") {
		return nil, fmt.Errorf("access token pool is empty")
	}
	client, err := newClient(token, "")
	if client == nil && retry > 0 {
		return New(token, retry-1)
	}
	return client, err
}

func newClient(token string, accountProxy string) (*Client, error) {
	// 保证 browserfp 全局 profile 已初始化，后续 proof/SO/header 共用。
	_ = browserfp.Get()

	appConf := conf.GetApp()
	baseURL := strings.TrimRight(appConf.ChatGPTBaseUrl, "/")
	if baseURL == "" {
		baseURL = "https://chatgpt.com"
	}

	authHeader := ""
	if strings.HasPrefix(token, "Bearer ") {
		authHeader = token
	} else if strings.HasPrefix(token, "eyJ") {
		authHeader = "Bearer " + token
	}
	deviceID, sessionID := resolveIdentity(authHeader)

	c := &Client{
		HTTP:      tls_client.NewClient(tls_client.NewClientOptions(300, common.GetClientProfile())),
		Auth:      &chatRequirements{},
		BaseURL:   baseURL,
		ChatURL:   baseURL + "/backend-anon/conversation",
		UserAgent: common.GetUa(),
		SessionID: sessionID,
		DeviceID:  deviceID,
		StartTime: time.Now(),
	}
	if c.HTTP == nil {
		return nil, fmt.Errorf("http client is nil")
	}
	if authHeader != "" {
		c.AccAuth = authHeader
		c.ChatURL = baseURL + "/backend-api/conversation"
	}
	proxy := strings.TrimSpace(accountProxy)
	if proxy == "" {
		proxy = strings.TrimSpace(appConf.Proxy)
	}
	if proxy != "" {
		if err := c.HTTP.SetProxy(proxy); err != nil {
			return nil, err
		}
	}
	// 若 token 来自账号池，自动回填 puid / team account id。
	if meta := token_pool.GetAccessTokenPool().FindByToken(c.AccAuth); meta != nil {
		applyAccountMeta(c, meta)
	}
	c.loadPowResources()
	if err := c.loadRequirements(); err != nil {
		return nil, err
	}
	return c, nil
}

func applyAccountMeta(c *Client, meta *token_pool.AccessToken) {
	if c == nil || meta == nil {
		return
	}
	if puid := strings.TrimSpace(meta.PUID); puid != "" {
		c.PUID = puid
	}
	if team := strings.TrimSpace(meta.TeamUserID); team != "" {
		c.TeamUserID = team
	}
	if proxy := strings.TrimSpace(meta.Proxy); proxy != "" && c.HTTP != nil {
		// proxy 已在 newClient 阶段设置；这里仅兜底空值场景。
		_ = proxy
	}
}

// Headers 对齐 aurora headerbuilder 基础浏览器头 + 鉴权/cookie。
func (c *Client) Headers(url string) (tls_client_httpi.Headers, tls_client_httpi.Cookies) {
	headers := tls_client_httpi.Headers{}
	headers.Set("accept", "*/*")
	headers.Set("accept-language", "en-US,en;q=0.9")
	headers.Set("oai-language", "en-US")
	headers.Set("origin", c.BaseURL)
	if c.ConversationID != "" {
		headers.Set("referer", c.BaseURL+"/c/"+c.ConversationID)
	} else {
		headers.Set("referer", c.BaseURL+"/")
	}
	headers.Set("priority", "u=1, i")
	headers.Set("sec-ch-ua", `"Chromium";v="148", "Google Chrome";v="148", "Not/A)Brand";v="99"`)
	headers.Set("sec-ch-ua-mobile", "?0")
	headers.Set("sec-ch-ua-platform", `"Windows"`)
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	headers.Set("user-agent", c.UserAgent)
	headers.Set("oai-device-id", c.DeviceID)
	headers.Set("oai-session-id", c.SessionID)
	headers.Set("oai-client-build-number", common.ClientBuildNumber)
	clientVersion := browserfp.DefaultBuildID
	if fp := browserfp.Get(); fp != nil && fp.BuildID != "" {
		clientVersion = fp.BuildID
	}
	if c.Pow.DataBuild != "" {
		clientVersion = c.Pow.DataBuild
	}
	headers.Set("oai-client-version", clientVersion)

	if c.AccAuth != "" {
		headers.Set("authorization", c.AccAuth)
	}
	if strings.TrimSpace(c.TeamUserID) != "" {
		headers.Set("chatgpt-account-id", strings.TrimSpace(c.TeamUserID))
	}

	cookies := append(tls_client_httpi.Cookies{}, c.Cookies...)
	if c.AccAuth == "" && c.DeviceID != "" {
		cookies = cookies.Append(&http.Cookie{Name: "oai-did", Value: c.DeviceID})
	}
	if strings.TrimSpace(c.PUID) != "" {
		cookies = cookies.Append(&http.Cookie{Name: "_puid", Value: strings.TrimSpace(c.PUID)})
	}
	return headers, cookies
}

func (c *Client) IsAuthenticated() bool {
	return c.AccAuth != ""
}

// ChatTimezone 与 fingerprint 时区保持一致（America/Los_Angeles）。
func (c *Client) ChatTimezone() (string, int) {
	return "America/Los_Angeles", 480
}

func (c *Client) TimeSinceLoadedSeconds() int {
	if c == nil || c.StartTime.IsZero() {
		return 0
	}
	sec := int(time.Since(c.StartTime).Seconds())
	if sec < 0 {
		return 0
	}
	return sec
}

func (c *Client) NoteConversation(conversationID string) {
	if c == nil {
		return
	}
	if strings.TrimSpace(conversationID) != "" {
		c.ConversationID = strings.TrimSpace(conversationID)
	}
}

func (c *Client) loadPowResources() {
	headers, cookies := c.Headers(c.BaseURL + "/")
	headers.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	headers.Set("sec-fetch-dest", "document")
	headers.Set("sec-fetch-mode", "navigate")
	headers.Set("sec-fetch-site", "none")
	headers.Set("sec-fetch-user", "?1")
	headers.Set("upgrade-insecure-requests", "1")
	response, err := c.HTTP.Request(tls_client_httpi.GET, c.BaseURL+"/", headers, cookies, nil)
	if err != nil || response == nil {
		c.Pow = Resources{ScriptSources: []string{defaultPowScript, sentinelSDKScript}, DataBuild: browserfp.DefaultBuildID}
		return
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	c.Pow = ParseResources(string(body))
	if len(c.Pow.ScriptSources) == 0 {
		c.Pow.ScriptSources = []string{defaultPowScript, sentinelSDKScript}
	}
	if c.Pow.DataBuild == "" {
		c.Pow.DataBuild = browserfp.DefaultBuildID
	}
}

func (c *Client) loadRequirements() error {
	requirementsToken := LegacyRequirementsToken(c.UserAgent, c.DeviceID, c.Pow)
	prepare, err := c.sentinelPrepare(requirementsToken)
	if err != nil {
		return err
	}
	if prepare.ForceLogin {
		common.SubUpdateThreshold()
		if c.AccAuth == "" {
			RotateNoAuthIdentity(c)
		}
		return fmt.Errorf("force login required")
	}
	if prepare.Arkose.Required {
		return fmt.Errorf("arkose token is required")
	}
	var proofToken string
	if prepare.ProofWork.Required {
		proofToken = CalcProofToken(prepare.ProofWork.Seed, prepare.ProofWork.Difficulty, c.UserAgent, c.DeviceID, c.Pow)
		if proofToken == "" {
			return fmt.Errorf("proof token calculation failed")
		}
	}
	var turnstileToken string
	if prepare.Turnstile.Required && prepare.Turnstile.Dx != "" {
		sourceP := ""
		if c.AccAuth == "" {
			sourceP = requirementsToken
		}
		turnstileToken = Solve(prepare.Turnstile.Dx, sourceP)
		if turnstileToken == "" {
			fallbackP := requirementsToken
			if sourceP == requirementsToken {
				fallbackP = ""
			}
			turnstileToken = Solve(prepare.Turnstile.Dx, fallbackP)
		}
	}

	// so 段：prepare 后异步跑 collector，业务发请求前 Snapshot + BuildToken。
	if prepare.So.Required && prepare.So.CollectorDX != "" && prepare.So.SnapshotDX != "" && prepare.Token != "" {
		c.soSession = so.NewSession(requirementsToken, prepare.So.CollectorDX)
		c.soSnapshotDX = prepare.So.SnapshotDX
		c.soChatToken = prepare.Token
		c.soFlow = c.resolveSOFlow()
		c.soSession.Start()
	}

	finalize, err := c.sentinelFinalize(prepare.PrepareToken, proofToken, turnstileToken)
	if err != nil {
		return err
	}
	if finalize.Token == "" {
		return fmt.Errorf("missing finalized sentinel token")
	}
	c.Auth.Token = finalize.Token
	c.Auth.PrepareToken = prepare.PrepareToken
	c.Auth.ProofWork.Ospt = proofToken
	c.Auth.TurnstileToken = turnstileToken
	c.Auth.So = prepare.So

	// /sentinel/req：best-effort，失败不阻断主链路。
	if reqToken, flow, reqErr := c.sentinelReq(); reqErr == nil {
		c.Auth.SentinelReqToken = reqToken
		c.Auth.SentinelReqFlow = flow
	}
	return nil
}

func (c *Client) resolveSOFlow() string {
	if c == nil {
		return "chatgpt"
	}
	if strings.TrimSpace(c.DeviceID) != "" {
		return c.DeviceID
	}
	if strings.TrimSpace(c.UserAgent) != "" {
		return "chatgpt-freeaccount"
	}
	return "chatgpt"
}

func (c *Client) soDeviceID() string {
	if c == nil {
		return ""
	}
	auth := strings.TrimSpace(c.AccAuth)
	if auth == "" {
		// noauth 对齐 aurora: 用 device/token uuid
		return strings.TrimSpace(c.DeviceID)
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

// EnsureSOToken 懒求值 openai-sentinel-so-token。
func (c *Client) EnsureSOToken() string {
	if c == nil {
		return ""
	}
	if c.soSession == nil {
		return c.SOToken
	}
	c.soOnce.Do(func() {
		soResult, err := c.soSession.Snapshot(c.soSnapshotDX)
		if err != nil {
			c.soErr = err
			return
		}
		c.soResult = soResult
	})
	if c.soErr != nil {
		return ""
	}
	if c.SOToken != "" {
		return c.SOToken
	}
	tok, err := so.BuildToken(c.soResult, c.soChatToken, c.soDeviceID(), c.soFlow)
	if err != nil {
		return ""
	}
	c.SOToken = tok
	return c.SOToken
}

func (c *Client) backendPath(path string) (authURL, targetPath string) {
	if c.AccAuth != "" {
		return c.BaseURL + "/backend-api" + path, "/backend-api" + path
	}
	return c.BaseURL + "/backend-anon" + path, "/backend-anon" + path
}

func (c *Client) sentinelPrepare(requirementsToken string) (*chatRequirements, error) {
	path := "/sentinel/chat-requirements/prepare"
	authURL, targetPath := c.backendPath(path)
	bodyJSON, err := json.Marshal(map[string]string{"p": requirementsToken})
	if err != nil {
		return nil, err
	}
	headers, cookies := c.Headers(authURL)
	headers.Set("content-type", "application/json")
	headers.Set("x-openai-target-path", targetPath)
	headers.Set("x-openai-target-route", targetPath)
	response, err := c.HTTP.Request(tls_client_httpi.POST, authURL, headers, cookies, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("sentinel prepare failed: status=%d body=%s", response.StatusCode, string(detail))
	}
	var result chatRequirements
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

type sentinelFinalizeResponse struct {
	Persona     string `json:"persona,omitempty"`
	Token       string `json:"token"`
	ExpireAfter int    `json:"expire_after,omitempty"`
}

func (c *Client) sentinelFinalize(prepareToken, proofToken, turnstileToken string) (*sentinelFinalizeResponse, error) {
	path := "/sentinel/chat-requirements/finalize"
	authURL, targetPath := c.backendPath(path)
	payload := map[string]string{"prepare_token": prepareToken}
	if proofToken != "" {
		payload["proofofwork"] = proofToken
	}
	if turnstileToken != "" {
		payload["turnstile"] = turnstileToken
	}
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	headers, cookies := c.Headers(authURL)
	headers.Set("content-type", "application/json")
	headers.Set("x-openai-target-path", targetPath)
	headers.Set("x-openai-target-route", targetPath)
	response, err := c.HTTP.Request(tls_client_httpi.POST, authURL, headers, cookies, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("sentinel finalize failed: status=%d body=%s", response.StatusCode, string(detail))
	}
	var result sentinelFinalizeResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

type sentinelReqResponse struct {
	Token     string `json:"token"`
	Flow      string `json:"flow"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	ChatReq   string `json:"chat_req,omitempty"`
	Persona   string `json:"persona,omitempty"`
}

func (c *Client) sentinelReq() (token string, flow string, err error) {
	path := "/sentinel/req"
	authURL, targetPath := c.backendPath(path)
	reqToken := RequirementsTokenNonce2(c.UserAgent, c.DeviceID, c.Pow)
	flow = "conversation"
	bodyJSON, err := json.Marshal(map[string]string{
		"p":    reqToken,
		"id":   c.DeviceID,
		"flow": flow,
	})
	if err != nil {
		return "", "", err
	}
	headers, cookies := c.Headers(authURL)
	headers.Set("accept", "*/*")
	headers.Set("content-type", "text/plain;charset=UTF-8")
	headers.Set("x-openai-target-path", targetPath)
	headers.Set("x-openai-target-route", targetPath)
	if c.ConversationID == "" {
		headers.Set("referer", c.BaseURL+"/backend-api/sentinel/frame.html?sv=20260423af3c")
	}
	response, err := c.HTTP.Request(tls_client_httpi.POST, authURL, headers, cookies, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return "", "", fmt.Errorf("sentinel req failed: status=%d body=%s", response.StatusCode, string(detail))
	}
	var result sentinelReqResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", "", err
	}
	if result.Flow != "" {
		flow = result.Flow
	}
	return result.Token, flow, nil
}

type sentinelExtraData struct {
	Version        int                  `json:"v"`
	SequenceNumber int                  `json:"sequence_number"`
	Signals        sentinelExtraSignals `json:"signals"`
	ConversationID string               `json:"conversation_id,omitempty"`
	LastMessageID  string               `json:"last_message_id,omitempty"`
}

type sentinelExtraSignals struct {
	PingSource                   string `json:"ping_source"`
	SOTokenPresent               string `json:"so_token_present"`
	TurnstileTokenPresent        string `json:"turnstile_token_present"`
	ProofTokenPresent            string `json:"proof_token_present"`
	PrepareTokenPresent          string `json:"prepare_token_present"`
	ChatRequirementsTokenPresent string `json:"chat_requirements_token_present"`
}

func boolToStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func (c *Client) buildSentinelExtraData(conversationID, lastMessageID, pingSource string, sequenceNumber int) string {
	if pingSource == "" {
		pingSource = "session_observer_background_submit"
	}
	soPresent := c.EnsureSOToken() != ""
	signals := sentinelExtraSignals{
		PingSource:                   pingSource,
		SOTokenPresent:               boolToStr(soPresent),
		TurnstileTokenPresent:        boolToStr(c.Auth != nil && c.Auth.TurnstileToken != ""),
		ProofTokenPresent:            boolToStr(c.Auth != nil && c.Auth.ProofWork.Ospt != ""),
		PrepareTokenPresent:          boolToStr(c.Auth != nil && c.Auth.PrepareToken != ""),
		ChatRequirementsTokenPresent: boolToStr(c.Auth != nil && c.Auth.Token != ""),
	}
	data := sentinelExtraData{
		Version:        1,
		SequenceNumber: sequenceNumber,
		Signals:        signals,
	}
	if conversationID != "" {
		data.ConversationID = "WEB:" + conversationID
	}
	if lastMessageID != "" {
		data.LastMessageID = lastMessageID
	}
	payload, _ := json.Marshal(data)
	return base64.StdEncoding.EncodeToString(payload)
}

// SentinelPing 对齐 aurora /sentinel/ping + Openai-Sentinel-Extra-Data。
func (c *Client) SentinelPing(conversationID, lastMessageID string) error {
	return c.SentinelPingWithSource(conversationID, lastMessageID, "session_observer_background_submit", 0)
}

func (c *Client) SentinelPingWithSource(conversationID, lastMessageID, pingSource string, sequenceNumber int) error {
	if c == nil || c.HTTP == nil {
		return fmt.Errorf("client is nil")
	}
	path := "/sentinel/ping"
	authURL, targetPath := c.backendPath(path)
	headers, cookies := c.Headers(authURL)
	headers.Set("x-openai-target-path", targetPath)
	headers.Set("x-openai-target-route", targetPath)
	if c.Auth != nil {
		if c.Auth.PrepareToken != "" {
			headers.Set("openai-sentinel-chat-requirements-prepare-token", c.Auth.PrepareToken)
		}
		if c.Auth.Token != "" {
			headers.Set("openai-sentinel-chat-requirements-token", c.Auth.Token)
		}
		if c.Auth.TurnstileToken != "" {
			headers.Set("openai-sentinel-turnstile-token", c.Auth.TurnstileToken)
		}
		if c.Auth.ProofWork.Ospt != "" {
			headers.Set("openai-sentinel-proof-token", c.Auth.ProofWork.Ospt)
		}
	}
	if soToken := c.EnsureSOToken(); soToken != "" {
		headers.Set("openai-sentinel-so-token", soToken)
	}
	headers.Set("openai-sentinel-extra-data", c.buildSentinelExtraData(conversationID, lastMessageID, pingSource, sequenceNumber))
	response, err := c.HTTP.Request(tls_client_httpi.POST, authURL, headers, cookies, nil)
	if err != nil {
		return fmt.Errorf("sentinel ping failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("sentinel ping failed: status=%d body=%s", response.StatusCode, string(detail))
	}
	return nil
}

// AsyncSentinelPing fire-and-forget ping，不阻塞主对话。
func (c *Client) AsyncSentinelPing(conversationID, lastMessageID string) {
	if c == nil {
		return
	}
	go func() {
		_ = c.SentinelPing(conversationID, lastMessageID)
	}()
}

func Retry() int {
	return constant.ReTry
}
