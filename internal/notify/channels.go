package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Channel interface {
	Type() string
	ValidateParams(params map[string]string) error
	Send(ctx context.Context, client *http.Client, params map[string]string, title, content string) error
}

var channelRegistry = map[string]Channel{
	"bark":      &BarkChannel{},
	"wecom_bot": &WeComBotChannel{},
	"wecom_app": &WeComAppChannel{},
	"feishu":    &FeishuChannel{},
	"telegram":  &TelegramChannel{},
	"pushplus":  &PushPlusChannel{},
	"custom":    &CustomWebhookChannel{},
}

func LookupChannel(channelType string) (Channel, bool) {
	ch, ok := channelRegistry[strings.ToLower(strings.TrimSpace(channelType))]
	return ch, ok
}

// ----------------------------------------------------
// 1. Bark
// ----------------------------------------------------
type BarkChannel struct{}

func (b *BarkChannel) Type() string { return "bark" }

func (b *BarkChannel) ValidateParams(params map[string]string) error {
	if strings.TrimSpace(params["bark_push"]) == "" {
		return fmt.Errorf("Bark 渠道需要填写 bark_push（推送 Key 或完整 URL）")
	}
	return nil
}

func (b *BarkChannel) Send(ctx context.Context, client *http.Client, params map[string]string, title, content string) error {
	rawURL := strings.TrimSpace(params["bark_push"])
	if rawURL == "" {
		return errors.New("missing bark_push")
	}
	reqURL := rawURL
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		reqURL = "https://api.day.app/" + rawURL
	}

	payload := map[string]interface{}{
		"title": title,
		"body":  content,
	}
	for paramKey, payloadKey := range map[string]string{
		"bark_archive": "isArchive",
		"bark_group":   "group",
		"bark_sound":   "sound",
		"bark_icon":    "icon",
		"bark_level":   "level",
		"bark_url":     "url",
	} {
		if v := strings.TrimSpace(params[paramKey]); v != "" {
			payload[payloadKey] = v
		}
	}

	body, status, err := postJSON(ctx, client, reqURL, nil, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("HTTP %d: %s", status, string(body))
	}
	var res struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body, &res); err == nil && res.Code != 0 && res.Code != 200 {
		return fmt.Errorf("bark code: %d, body: %s", res.Code, string(body))
	}
	return nil
}

// ----------------------------------------------------
// 2. 企业微信机器人 (WeCom Bot)
// ----------------------------------------------------
type WeComBotChannel struct{}

func (w *WeComBotChannel) Type() string { return "wecom_bot" }

func (w *WeComBotChannel) ValidateParams(params map[string]string) error {
	if strings.TrimSpace(params["qywx_key"]) == "" {
		return fmt.Errorf("企业微信机器人需要填写 qywx_key")
	}
	return nil
}

func (w *WeComBotChannel) Send(ctx context.Context, client *http.Client, params map[string]string, title, content string) error {
	key := strings.TrimSpace(params["qywx_key"])
	if key == "" {
		return errors.New("missing qywx_key")
	}
	origin := strings.TrimRight(strings.TrimSpace(params["qywx_origin"]), "/")
	if origin == "" {
		origin = "https://qyapi.weixin.qq.com"
	}
	reqURL := fmt.Sprintf("%s/cgi-bin/webhook/send?key=%s", origin, url.QueryEscape(key))

	payload := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": fmt.Sprintf("%s\n\n%s", title, content),
		},
	}

	body, status, err := postJSON(ctx, client, reqURL, nil, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("HTTP %d: %s", status, string(body))
	}
	var res struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return err
	}
	if res.ErrCode != 0 {
		return fmt.Errorf("wecom_bot errcode: %d, msg: %s", res.ErrCode, res.ErrMsg)
	}
	return nil
}

// ----------------------------------------------------
// 3. 企业微信应用 (WeCom App)
// ----------------------------------------------------
type WeComAppChannel struct{}

func (w *WeComAppChannel) Type() string { return "wecom_app" }

func (w *WeComAppChannel) ValidateParams(params map[string]string) error {
	parts := strings.Split(strings.TrimSpace(params["qywx_am"]), ",")
	if len(parts) < 4 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" ||
		strings.TrimSpace(parts[2]) == "" || strings.TrimSpace(parts[3]) == "" {
		return fmt.Errorf("企业微信应用需要填写 qywx_am（格式: corpid,corpsecret,touser,agentid[,media_id]）")
	}
	return nil
}

func (w *WeComAppChannel) Send(ctx context.Context, client *http.Client, params map[string]string, title, content string) error {
	origin := strings.TrimRight(strings.TrimSpace(params["qywx_origin"]), "/")
	if origin == "" {
		origin = "https://qyapi.weixin.qq.com"
	}
	parts := strings.Split(strings.TrimSpace(params["qywx_am"]), ",")
	if len(parts) < 4 {
		return errors.New("qywx_am format error: expected corpid,corpsecret,touser,agentid[,media_id]")
	}
	corpID := strings.TrimSpace(parts[0])
	corpSecret := strings.TrimSpace(parts[1])
	toUser := strings.TrimSpace(parts[2])
	agentID := strings.TrimSpace(parts[3])
	mediaID := ""
	if len(parts) >= 5 {
		mediaID = strings.TrimSpace(parts[4])
	}

	tokenURL := fmt.Sprintf("%s/cgi-bin/gettoken?corpid=%s&corpsecret=%s", origin, url.QueryEscape(corpID), url.QueryEscape(corpSecret))
	tokenResp, _, err := getRequest(ctx, client, tokenURL)
	if err != nil {
		return fmt.Errorf("gettoken failed: %w", err)
	}
	var tokenRes struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(tokenResp, &tokenRes); err != nil {
		return err
	}
	if tokenRes.AccessToken == "" {
		return fmt.Errorf("gettoken error: %d, %s", tokenRes.ErrCode, tokenRes.ErrMsg)
	}

	sendURL := fmt.Sprintf("%s/cgi-bin/message/send?access_token=%s", origin, url.QueryEscape(tokenRes.AccessToken))
	var payload map[string]interface{}
	if mediaID == "" {
		payload = map[string]interface{}{
			"touser":  toUser,
			"msgtype": "text",
			"agentid": agentID,
			"text": map[string]string{
				"content": fmt.Sprintf("%s\n\n%s", title, content),
			},
			"safe": "0",
		}
	} else {
		payload = map[string]interface{}{
			"touser":  toUser,
			"msgtype": "mpnews",
			"agentid": agentID,
			"mpnews": map[string]interface{}{
				"articles": []map[string]string{
					{
						"title":          title,
						"thumb_media_id": mediaID,
						"author":         "cloud-vm-manager",
						"content":        strings.ReplaceAll(content, "\n", "<br/>"),
						"digest":         content,
					},
				},
			},
		}
	}

	sendResp, status, err := postJSON(ctx, client, sendURL, nil, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("HTTP %d: %s", status, string(sendResp))
	}
	var res struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(sendResp, &res); err != nil {
		return err
	}
	if res.ErrCode != 0 {
		return fmt.Errorf("wecom_app errcode: %d, msg: %s", res.ErrCode, res.ErrMsg)
	}
	return nil
}

// ----------------------------------------------------
// 4. 飞书机器人 (Feishu)
// ----------------------------------------------------
type FeishuChannel struct{}

func (f *FeishuChannel) Type() string { return "feishu" }

func (f *FeishuChannel) ValidateParams(params map[string]string) error {
	if strings.TrimSpace(params["fskey"]) == "" {
		return fmt.Errorf("飞书机器人需要填写 fskey")
	}
	return nil
}

func (f *FeishuChannel) Send(ctx context.Context, client *http.Client, params map[string]string, title, content string) error {
	fskey := strings.TrimSpace(params["fskey"])
	if fskey == "" {
		return errors.New("missing fskey")
	}
	reqURL := fskey
	if !strings.HasPrefix(fskey, "http://") && !strings.HasPrefix(fskey, "https://") {
		reqURL = fmt.Sprintf("https://open.feishu.cn/open-apis/bot/v2/hook/%s", fskey)
	}
	payload := map[string]interface{}{
		"msg_type": "text",
		"content": map[string]string{
			"text": fmt.Sprintf("%s\n\n%s", title, content),
		},
	}

	if secret := strings.TrimSpace(params["fssecret"]); secret != "" {
		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		stringToSign := timestamp + "\n" + secret
		hmacCode := hmacSha256([]byte(stringToSign), []byte(""))
		payload["timestamp"] = timestamp
		payload["sign"] = base64.StdEncoding.EncodeToString(hmacCode)
	}

	body, status, err := postJSON(ctx, client, reqURL, nil, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("HTTP %d: %s", status, string(body))
	}
	var res struct {
		Code       int    `json:"code"`
		StatusCode int    `json:"StatusCode"`
		Msg        string `json:"msg"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return err
	}
	if res.Code != 0 && res.StatusCode != 0 {
		return fmt.Errorf("feishu code: %d, msg: %s", res.Code, res.Msg)
	}
	return nil
}

// ----------------------------------------------------
// 5. Telegram (单向推送)
// ----------------------------------------------------
type TelegramChannel struct{}

func (t *TelegramChannel) Type() string { return "telegram" }

func (t *TelegramChannel) ValidateParams(params map[string]string) error {
	if strings.TrimSpace(params["tg_bot_token"]) == "" {
		return fmt.Errorf("Telegram 渠道需要填写 tg_bot_token")
	}
	if strings.TrimSpace(params["tg_user_id"]) == "" {
		return fmt.Errorf("Telegram 渠道需要填写 tg_user_id（目标 Chat/User ID）")
	}
	return nil
}

func (t *TelegramChannel) Send(ctx context.Context, client *http.Client, params map[string]string, title, content string) error {
	token := strings.TrimSpace(params["tg_bot_token"])
	chatID := strings.TrimSpace(params["tg_user_id"])
	if token == "" || chatID == "" {
		return errors.New("missing tg_bot_token or tg_user_id")
	}
	apiHost := strings.TrimRight(strings.TrimSpace(params["tg_api_host"]), "/")
	if apiHost == "" {
		apiHost = "https://api.telegram.org"
	}

	return SendTelegramText(ctx, client, apiHost, token, chatID, fmt.Sprintf("%s\n\n%s", title, content), "")
}

func SendTelegramText(ctx context.Context, client *http.Client, apiHost, token, chatID, text, parseMode string) error {
	if apiHost == "" {
		apiHost = "https://api.telegram.org"
	}
	reqURL := fmt.Sprintf("%s/bot%s/sendMessage", strings.TrimRight(apiHost, "/"), token)
	payload := map[string]interface{}{
		"chat_id":                  chatID,
		"text":                     truncateRunes(text, 4000),
		"disable_web_page_preview": true,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}

	body, status, err := postJSON(ctx, client, reqURL, nil, payload)
	if err != nil {
		return err
	}
	var res struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		if status < 200 || status >= 300 {
			return fmt.Errorf("HTTP %d: %s", status, string(body))
		}
		return err
	}
	if !res.OK {
		return fmt.Errorf("telegram error: %s", res.Description)
	}
	return nil
}

// ----------------------------------------------------
// 6. PushPlus
// ----------------------------------------------------
type PushPlusChannel struct{}

func (p *PushPlusChannel) Type() string { return "pushplus" }

func (p *PushPlusChannel) ValidateParams(params map[string]string) error {
	if strings.TrimSpace(params["push_plus_token"]) == "" {
		return fmt.Errorf("PushPlus 渠道需要填写 push_plus_token")
	}
	return nil
}

func (p *PushPlusChannel) Send(ctx context.Context, client *http.Client, params map[string]string, title, content string) error {
	token := strings.TrimSpace(params["push_plus_token"])
	if token == "" {
		return errors.New("missing push_plus_token")
	}
	template := strings.TrimSpace(params["push_plus_template"])
	if template == "" {
		template = "txt"
	}
	channel := strings.TrimSpace(params["push_plus_channel"])
	if channel == "" {
		channel = "wechat"
	}
	payload := map[string]interface{}{
		"token":       token,
		"title":       title,
		"content":     content,
		"topic":       strings.TrimSpace(params["push_plus_user"]),
		"template":    template,
		"channel":     channel,
		"webhook":     strings.TrimSpace(params["push_plus_webhook"]),
		"callbackUrl": strings.TrimSpace(params["push_plus_callbackurl"]),
		"to":          strings.TrimSpace(params["push_plus_to"]),
	}

	body, status, err := postJSON(ctx, client, "https://www.pushplus.plus/send", nil, payload)
	if err == nil && status >= 200 && status < 300 {
		var res struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(body, &res); err == nil && res.Code == 200 {
			return nil
		}
	}

	body, status, err = postJSON(ctx, client, "http://pushplus.hxtrip.com/send", nil, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("HTTP %d: %s", status, string(body))
	}
	var res struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return err
	}
	if res.Code != 200 {
		return fmt.Errorf("pushplus code: %d, msg: %s", res.Code, res.Msg)
	}
	return nil
}

// ----------------------------------------------------
// 7. 自定义 Webhook (Custom)
// ----------------------------------------------------
type CustomWebhookChannel struct{}

func (c *CustomWebhookChannel) Type() string { return "custom" }

func (c *CustomWebhookChannel) ValidateParams(params map[string]string) error {
	if strings.TrimSpace(params["webhook_url"]) == "" {
		return fmt.Errorf("自定义 Webhook 需要填写 webhook_url")
	}
	method := strings.ToUpper(strings.TrimSpace(params["webhook_method"]))
	if method == "" {
		method = http.MethodPost
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return fmt.Errorf("不支持的 Webhook 请求方法 %q", method)
	}
	return nil
}

func (c *CustomWebhookChannel) Send(ctx context.Context, client *http.Client, params map[string]string, title, content string) error {
	rawURL := strings.TrimSpace(params["webhook_url"])
	if rawURL == "" {
		return errors.New("missing webhook_url")
	}
	method := strings.ToUpper(strings.TrimSpace(params["webhook_method"]))
	if method == "" {
		method = http.MethodPost
	}
	rawBody := params["webhook_body"]
	contentType := strings.TrimSpace(params["webhook_content_type"])
	rawHeaders := params["webhook_headers"]

	formattedURL := strings.ReplaceAll(rawURL, "$title", url.QueryEscape(title))
	formattedURL = strings.ReplaceAll(formattedURL, "$content", url.QueryEscape(content))

	var reqBody io.Reader
	if strings.TrimSpace(rawBody) != "" {
		replaced := replaceTemplateVars(rawBody, contentType, title, content)
		reqBody = strings.NewReader(replaced)
	}

	req, err := http.NewRequestWithContext(ctx, method, formattedURL, reqBody)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if rawHeaders != "" {
		for _, line := range strings.Split(rawHeaders, "\n") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				if k != "" {
					req.Header.Set(k, strings.TrimSpace(parts[1]))
				}
			}
		}
	}

	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func replaceTemplateVars(rawBody, contentType, title, content string) string {
	isJSON := strings.Contains(strings.ToLower(contentType), "json") ||
		strings.HasPrefix(strings.TrimSpace(rawBody), "{") ||
		strings.HasPrefix(strings.TrimSpace(rawBody), "[")
	if isJSON {
		safeTitle := jsonStringInner(title)
		safeContent := jsonStringInner(content)
		out := strings.ReplaceAll(rawBody, "$title", safeTitle)
		return strings.ReplaceAll(out, "$content", safeContent)
	}
	out := strings.ReplaceAll(rawBody, "$title", title)
	return strings.ReplaceAll(out, "$content", content)
}

func jsonStringInner(s string) string {
	b, err := json.Marshal(s)
	if err != nil || len(b) < 2 {
		return s
	}
	return string(b[1 : len(b)-1])
}

func hmacSha256(key, message []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(message)
	return h.Sum(nil)
}

func getRequest(ctx context.Context, client *http.Client, reqURL string) ([]byte, int, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return body, resp.StatusCode, err
}

func postJSON(ctx context.Context, client *http.Client, reqURL string, headers map[string]string, payload interface{}) ([]byte, int, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json;charset=utf-8")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return respBody, resp.StatusCode, err
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
