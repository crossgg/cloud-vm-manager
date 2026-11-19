package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type apiClient struct {
	httpClient *http.Client
	apiHost    string
	botToken   string
}

func newAPIClient(httpClient *http.Client, apiHost, botToken string) *apiClient {
	host := strings.TrimRight(strings.TrimSpace(apiHost), "/")
	if host == "" {
		host = "https://api.telegram.org"
	}
	return &apiClient{
		httpClient: httpClient,
		apiHost:    host,
		botToken:   strings.TrimSpace(botToken),
	}
}

type tgUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type tgChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type tgMessage struct {
	MessageID int64   `json:"message_id"`
	From      *tgUser `json:"from"`
	Chat      tgChat  `json:"chat"`
	Text      string  `json:"text"`
}

type tgCallbackQuery struct {
	ID      string     `json:"id"`
	From    tgUser     `json:"from"`
	Message *tgMessage `json:"message"`
	Data    string     `json:"data"`
}

type tgUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *tgMessage       `json:"message"`
	CallbackQuery *tgCallbackQuery `json:"callback_query"`
}

type inlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

type inlineKeyboardMarkup struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

type botCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

func (c *apiClient) endpoint(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", c.apiHost, c.botToken, method)
}

func (c *apiClient) doJSON(ctx context.Context, method string, payload interface{}, out interface{}) error {
	var bodyReader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader([]byte("{}"))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(method), bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}

	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
		ErrorCode   int             `json:"error_code"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("telegram %s decode failed (HTTP %d): %w", method, resp.StatusCode, err)
	}
	if !envelope.OK {
		return fmt.Errorf("telegram %s error (%d): %s", method, envelope.ErrorCode, envelope.Description)
	}
	if out != nil && len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("telegram %s result decode failed: %w", method, err)
		}
	}
	return nil
}

func (c *apiClient) GetMe(ctx context.Context) (*tgUser, error) {
	var user tgUser
	if err := c.doJSON(ctx, "getMe", nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *apiClient) SetMyCommands(ctx context.Context, commands []botCommand) error {
	return c.doJSON(ctx, "setMyCommands", map[string]interface{}{
		"commands": commands,
	}, nil)
}

func (c *apiClient) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]tgUpdate, error) {
	var updates []tgUpdate
	payload := map[string]interface{}{
		"offset":          offset,
		"timeout":         timeoutSec,
		"allowed_updates": []string{"message", "callback_query"},
	}
	if err := c.doJSON(ctx, "getUpdates", payload, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *apiClient) SendMessage(ctx context.Context, chatID int64, htmlText string, markup *inlineKeyboardMarkup) error {
	payload := map[string]interface{}{
		"chat_id":                  chatID,
		"text":                     truncateRunes(htmlText, 3950),
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}
	return c.doJSON(ctx, "sendMessage", payload, nil)
}

func (c *apiClient) EditMessageText(ctx context.Context, chatID, messageID int64, htmlText string, markup *inlineKeyboardMarkup) error {
	payload := map[string]interface{}{
		"chat_id":                  chatID,
		"message_id":               messageID,
		"text":                     truncateRunes(htmlText, 3950),
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}
	err := c.doJSON(ctx, "editMessageText", payload, nil)
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	return err
}

func (c *apiClient) AnswerCallbackQuery(ctx context.Context, callbackID, text string, showAlert bool) error {
	payload := map[string]interface{}{
		"callback_query_id": callbackID,
	}
	if text != "" {
		payload["text"] = truncateRunes(text, 180)
	}
	if showAlert {
		payload["show_alert"] = true
	}
	return c.doJSON(ctx, "answerCallbackQuery", payload, nil)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n...(已截断)"
}
