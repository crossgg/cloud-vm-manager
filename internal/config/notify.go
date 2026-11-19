package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	NotifyEventOCITaskSucceeded  = "oci_task_succeeded"
	NotifyEventOCIDTAlert        = "oci_dt_alert"
	NotifyEventAzureCreditAlert  = "azure_credit_alert"
	NotifyEventVMChangeIP        = "vm_change_ip"
	NotifyEventVMPower           = "vm_power"
	NotifyEventAuthLogin         = "auth_login"
)

var notifyConfigMu sync.Mutex

type NotifyEventDefinition struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	DefaultOn   bool   `json:"defaultOn"`
}

func AllNotifyEventDefinitions() []NotifyEventDefinition {
	return []NotifyEventDefinition{
		{
			ID:          NotifyEventOCITaskSucceeded,
			Label:       "OCI 抢购成功",
			Description: "OCI 创建/抢购任务成功创建实例时通知（失败重试不通知）",
			DefaultOn:   true,
		},
		{
			ID:          NotifyEventOCIDTAlert,
			Label:       "OCI 流量告警",
			Description: "OCI 出站流量达到阈值告警或触发自动停机时通知",
			DefaultOn:   true,
		},
		{
			ID:          NotifyEventAzureCreditAlert,
			Label:       "Azure 赠金/费用告警",
			Description: "Azure 赠金监控触发剩余额度/到期/月消费阈值或自动动作时通知",
			DefaultOn:   true,
		},
		{
			ID:          NotifyEventVMChangeIP,
			Label:       "更换公网 IP",
			Description: "通过面板或 TG Bot 更换实例公网 IP 完成时通知",
			DefaultOn:   true,
		},
		{
			ID:          NotifyEventVMPower,
			Label:       "实例开关机 / 重启",
			Description: "实例执行开机、关机或重启成功时通知",
			DefaultOn:   false,
		},
		{
			ID:          NotifyEventAuthLogin,
			Label:       "面板登录提醒",
			Description: "面板登录成功或连续失败触发锁定时通知",
			DefaultOn:   true,
		},
	}
}

func IsSupportedNotifyEvent(eventType string) bool {
	eventType = strings.TrimSpace(strings.ToLower(eventType))
	for _, def := range AllNotifyEventDefinitions() {
		if def.ID == eventType {
			return true
		}
	}
	return false
}

func DefaultEnabledNotifyEvents() []string {
	var out []string
	for _, def := range AllNotifyEventDefinitions() {
		if def.DefaultOn {
			out = append(out, def.ID)
		}
	}
	return out
}

func SupportedNotifyChannelTypes() []string {
	return []string{"bark", "wecom_bot", "wecom_app", "feishu", "telegram", "pushplus", "custom"}
}

func IsSupportedNotifyChannelType(channelType string) bool {
	t := strings.TrimSpace(strings.ToLower(channelType))
	for _, item := range SupportedNotifyChannelTypes() {
		if item == t {
			return true
		}
	}
	return false
}

type TelegramBotConfig struct {
	Enabled           bool     `json:"enabled"`
	BotToken          string   `json:"botToken"`
	APIHost           string   `json:"apiHost"`
	ProxyID           string   `json:"proxyId"`
	FallbackProxyID   string   `json:"fallbackProxyId"`
	AllowedUserIDs    []int64  `json:"allowedUserIds"`
	AllowedChatIDs    []int64  `json:"allowedChatIds"`
	AutoNotify        bool     `json:"autoNotify"`
	NotifyChatIDs     []int64  `json:"notifyChatIds"`
	NotifyEvents      []string `json:"notifyEvents"`
	NotifyCooldownSec int      `json:"notifyCooldownSec"`
}

func (b TelegramBotConfig) EffectiveNotifyChatIDs() []int64 {
	if len(b.NotifyChatIDs) > 0 {
		return append([]int64(nil), b.NotifyChatIDs...)
	}
	if len(b.AllowedChatIDs) > 0 {
		return append([]int64(nil), b.AllowedChatIDs...)
	}
	return append([]int64(nil), b.AllowedUserIDs...)
}

type NotifyChannelConfig struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	Enabled         bool              `json:"enabled"`
	ProxyID         string            `json:"proxyId"`
	FallbackProxyID string            `json:"fallbackProxyId"`
	CooldownSec     int               `json:"cooldownSec"`
	Events          []string          `json:"events"`
	Accounts        []string          `json:"accounts"`
	Params          map[string]string `json:"params"`
}

type NotifyConfig struct {
	EnabledEvents []string              `json:"enabledEvents"`
	TelegramBot   TelegramBotConfig     `json:"telegramBot"`
	Channels      []NotifyChannelConfig `json:"channels"`
}

func DefaultNotifyConfig() NotifyConfig {
	return NotifyConfig{
		EnabledEvents: DefaultEnabledNotifyEvents(),
		TelegramBot: TelegramBotConfig{
			APIHost:           "https://api.telegram.org",
			ProxyID:           ProxyFallbackNone,
			FallbackProxyID:   ProxyFallbackNone,
			AutoNotify:        true,
			NotifyEvents:      DefaultEnabledNotifyEvents(),
			NotifyCooldownSec: 300,
		},
		Channels: nil,
	}
}

func (n NotifyConfig) Normalize() NotifyConfig {
	out := NotifyConfig{
		EnabledEvents: normalizeNotifyEvents(n.EnabledEvents),
		TelegramBot: TelegramBotConfig{
			Enabled:           n.TelegramBot.Enabled,
			BotToken:          strings.TrimSpace(n.TelegramBot.BotToken),
			APIHost:           strings.TrimRight(strings.TrimSpace(n.TelegramBot.APIHost), "/"),
			ProxyID:           normalizeNotifyProxyID(n.TelegramBot.ProxyID),
			FallbackProxyID:   NormalizeProxyFallback(n.TelegramBot.FallbackProxyID),
			AllowedUserIDs:    dedupeInt64Slice(n.TelegramBot.AllowedUserIDs),
			AllowedChatIDs:    dedupeInt64Slice(n.TelegramBot.AllowedChatIDs),
			AutoNotify:        n.TelegramBot.AutoNotify,
			NotifyChatIDs:     dedupeInt64Slice(n.TelegramBot.NotifyChatIDs),
			NotifyEvents:      normalizeNotifyEvents(n.TelegramBot.NotifyEvents),
			NotifyCooldownSec: n.TelegramBot.NotifyCooldownSec,
		},
		Channels: make([]NotifyChannelConfig, 0, len(n.Channels)),
	}
	if out.TelegramBot.APIHost == "" {
		out.TelegramBot.APIHost = "https://api.telegram.org"
	}
	if out.TelegramBot.NotifyCooldownSec < 0 {
		out.TelegramBot.NotifyCooldownSec = 0
	}
	for _, ch := range n.Channels {
		params := make(map[string]string, len(ch.Params))
		for k, v := range ch.Params {
			nk := strings.ToLower(strings.TrimSpace(k))
			if nk == "" {
				continue
			}
			params[nk] = v
		}
		cooldown := ch.CooldownSec
		if cooldown < 0 {
			cooldown = 0
		}
		out.Channels = append(out.Channels, NotifyChannelConfig{
			ID:              strings.TrimSpace(ch.ID),
			Name:            strings.TrimSpace(ch.Name),
			Type:            strings.ToLower(strings.TrimSpace(ch.Type)),
			Enabled:         ch.Enabled,
			ProxyID:         normalizeNotifyProxyID(ch.ProxyID),
			FallbackProxyID: NormalizeProxyFallback(ch.FallbackProxyID),
			CooldownSec:     cooldown,
			Events:          normalizeNotifyEvents(ch.Events),
			Accounts:        normalizeStringList(ch.Accounts),
			Params:          params,
		})
	}
	return out
}

func NotifyConfigPath(mainPath string) string {
	if strings.HasPrefix(mainPath, "config/") || strings.HasPrefix(mainPath, "config\\") {
		return "config/notify.conf"
	}
	return "notify.conf"
}

func LoadNotifyConfig(path string, cfg *Config) error {
	cfg.Notify = DefaultNotifyConfig()
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	sections := map[string]map[string]map[string]string{
		"triggers":       {},
		"telegram_bot":   {},
		"notify_channel": {},
	}
	sectionOrder := map[string][]string{
		"triggers":       {},
		"telegram_bot":   {},
		"notify_channel": {},
	}

	currentBlock := ""
	currentSection := ""
	for lineNo, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasSuffix(lower, "=begin") {
			currentBlock = strings.TrimSpace(strings.TrimSuffix(lower, "=begin"))
			currentSection = ""
			if _, ok := sections[currentBlock]; !ok {
				return fmt.Errorf("line %d: unsupported block %q", lineNo+1, currentBlock)
			}
			continue
		}
		if strings.HasSuffix(lower, "=end") {
			endBlock := strings.TrimSpace(strings.TrimSuffix(lower, "=end"))
			if currentBlock != endBlock {
				return fmt.Errorf("line %d: mismatched end block %q", lineNo+1, endBlock)
			}
			currentBlock = ""
			currentSection = ""
			continue
		}
		if currentBlock == "" {
			return fmt.Errorf("line %d: setting outside block", lineNo+1)
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if currentSection == "" {
				return fmt.Errorf("line %d: empty section name", lineNo+1)
			}
			if _, exists := sections[currentBlock][currentSection]; exists {
				return fmt.Errorf("line %d: duplicate section %q", lineNo+1, currentSection)
			}
			sections[currentBlock][currentSection] = map[string]string{}
			sectionOrder[currentBlock] = append(sectionOrder[currentBlock], currentSection)
			continue
		}
		if currentSection == "" {
			return fmt.Errorf("line %d: setting before section", lineNo+1)
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("line %d: invalid setting", lineNo+1)
		}
		normKey := strings.ToLower(strings.TrimSpace(key))
		sections[currentBlock][currentSection][normKey] = strings.Trim(strings.TrimSpace(value), `"`)
	}

	loaded := DefaultNotifyConfig()
	if len(sectionOrder["triggers"]) > 0 {
		values := sections["triggers"][sectionOrder["triggers"][0]]
		if raw, exists := values["enabled_events"]; exists {
			loaded.EnabledEvents = parseCSVStrings(raw)
		}
	}

	if len(sectionOrder["telegram_bot"]) > 0 {
		values := sections["telegram_bot"][sectionOrder["telegram_bot"][0]]
		loaded.TelegramBot.Enabled = boolValue(values["enabled"])
		loaded.TelegramBot.BotToken = values["bot_token"]
		if v := strings.TrimSpace(values["api_host"]); v != "" {
			loaded.TelegramBot.APIHost = v
		}
		if v, ok := values["proxy"]; ok {
			loaded.TelegramBot.ProxyID = v
		}
		if v, ok := values["fallback"]; ok {
			loaded.TelegramBot.FallbackProxyID = v
		}
		if v, ok := values["allowed_user_ids"]; ok {
			ids, err := parseCSVInt64(v)
			if err != nil {
				return fmt.Errorf("telegram_bot allowed_user_ids: %w", err)
			}
			loaded.TelegramBot.AllowedUserIDs = ids
		}
		if v, ok := values["allowed_chat_ids"]; ok {
			ids, err := parseCSVInt64(v)
			if err != nil {
				return fmt.Errorf("telegram_bot allowed_chat_ids: %w", err)
			}
			loaded.TelegramBot.AllowedChatIDs = ids
		}
		if v, ok := values["auto_notify"]; ok {
			loaded.TelegramBot.AutoNotify = boolValue(v)
		}
		if v, ok := values["notify_chat_ids"]; ok {
			ids, err := parseCSVInt64(v)
			if err != nil {
				return fmt.Errorf("telegram_bot notify_chat_ids: %w", err)
			}
			loaded.TelegramBot.NotifyChatIDs = ids
		}
		if v, ok := values["notify_events"]; ok {
			loaded.TelegramBot.NotifyEvents = parseCSVStrings(v)
		}
		if v, ok := values["notify_cooldown_sec"]; ok {
			loaded.TelegramBot.NotifyCooldownSec = intValueOrDefault(v, 300)
		}
	}

	for _, id := range sectionOrder["notify_channel"] {
		values := sections["notify_channel"][id]
		ch := NotifyChannelConfig{
			ID:              id,
			Name:            valueOrDefault(values["name"], id),
			Type:            strings.ToLower(values["type"]),
			Enabled:         boolValue(values["enabled"]),
			ProxyID:         values["proxy"],
			FallbackProxyID: values["fallback"],
			CooldownSec:     intValueOrDefault(values["cooldown_sec"], 300),
			Events:          parseCSVStrings(values["events"]),
			Accounts:        parseCSVStrings(values["accounts"]),
			Params:          map[string]string{},
		}
		for k, v := range values {
			if strings.HasPrefix(k, "param.") {
				paramKey := strings.TrimPrefix(k, "param.")
				if paramKey != "" {
					ch.Params[paramKey] = unescapeNotifyLineValue(v)
				}
			}
		}
		loaded.Channels = append(loaded.Channels, ch)
	}

	loaded = loaded.Normalize()
	if err := ValidateNotifyConfig(loaded); err != nil {
		return err
	}
	cfg.Notify = loaded
	return nil
}

func SaveNotifyConfig(path string, notifyCfg NotifyConfig) error {
	notifyConfigMu.Lock()
	defer notifyConfigMu.Unlock()

	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("notify config path is empty")
	}
	normalized := notifyCfg.Normalize()
	if err := ValidateNotifyConfig(normalized); err != nil {
		return err
	}

	var lines []string
	lines = append(lines,
		"triggers=begin",
		"[global]",
		"enabled_events="+strings.Join(normalized.EnabledEvents, ","),
		"triggers=end",
		"",
		"telegram_bot=begin",
		"[default]",
		"enabled="+strconv.FormatBool(normalized.TelegramBot.Enabled),
		"bot_token="+normalized.TelegramBot.BotToken,
		"api_host="+normalized.TelegramBot.APIHost,
		"proxy="+normalized.TelegramBot.ProxyID,
		"fallback="+NormalizeProxyFallback(normalized.TelegramBot.FallbackProxyID),
		"allowed_user_ids="+joinInt64CSV(normalized.TelegramBot.AllowedUserIDs),
		"allowed_chat_ids="+joinInt64CSV(normalized.TelegramBot.AllowedChatIDs),
		"auto_notify="+strconv.FormatBool(normalized.TelegramBot.AutoNotify),
		"notify_chat_ids="+joinInt64CSV(normalized.TelegramBot.NotifyChatIDs),
		"notify_events="+strings.Join(normalized.TelegramBot.NotifyEvents, ","),
		"notify_cooldown_sec="+strconv.Itoa(normalized.TelegramBot.NotifyCooldownSec),
		"telegram_bot=end",
		"",
		"notify_channel=begin",
	)

	for _, ch := range normalized.Channels {
		lines = append(lines,
			"["+ch.ID+"]",
			"name="+ch.Name,
			"type="+ch.Type,
			"enabled="+strconv.FormatBool(ch.Enabled),
			"proxy="+ch.ProxyID,
			"fallback="+NormalizeProxyFallback(ch.FallbackProxyID),
			"cooldown_sec="+strconv.Itoa(ch.CooldownSec),
			"events="+strings.Join(ch.Events, ","),
			"accounts="+strings.Join(ch.Accounts, ","),
		)
		paramKeys := make([]string, 0, len(ch.Params))
		for k := range ch.Params {
			paramKeys = append(paramKeys, k)
		}
		sort.Strings(paramKeys)
		for _, k := range paramKeys {
			lines = append(lines, "param."+k+"="+escapeNotifyLineValue(ch.Params[k]))
		}
	}
	lines = append(lines, "notify_channel=end", "")

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return writeFileAtomic(path, []byte(strings.Join(lines, "\n")), 0600)
}

func ValidateNotifyConfig(cfg NotifyConfig) error {
	for _, ev := range cfg.EnabledEvents {
		if !IsSupportedNotifyEvent(ev) {
			return fmt.Errorf("unsupported notify event %q", ev)
		}
	}
	bot := cfg.TelegramBot
	if HasUnsafeConfigLineValue(bot.BotToken) || HasUnsafeConfigLineValue(bot.APIHost) {
		return fmt.Errorf("telegram bot configuration contains unsupported characters")
	}
	if bot.Enabled && strings.TrimSpace(bot.BotToken) == "" {
		return fmt.Errorf("启用交互 Telegram Bot 时必须填写 Bot Token")
	}
	for _, ev := range bot.NotifyEvents {
		if !IsSupportedNotifyEvent(ev) {
			return fmt.Errorf("telegram bot unsupported notify event %q", ev)
		}
	}

	seenIDs := make(map[string]struct{}, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		if ch.ID == "" || HasUnsafeConfigValue(ch.ID) || strings.ContainsAny(ch.ID, `/\\`) {
			return fmt.Errorf("invalid notify channel id %q", ch.ID)
		}
		if _, exists := seenIDs[ch.ID]; exists {
			return fmt.Errorf("duplicate notify channel id %q", ch.ID)
		}
		seenIDs[ch.ID] = struct{}{}
		if !IsSupportedNotifyChannelType(ch.Type) {
			return fmt.Errorf("notify channel %s: unsupported type %q", ch.ID, ch.Type)
		}
		if HasUnsafeConfigLineValue(ch.Name) {
			return fmt.Errorf("notify channel %s: name contains unsupported characters", ch.ID)
		}
		for _, ev := range ch.Events {
			if !IsSupportedNotifyEvent(ev) {
				return fmt.Errorf("notify channel %s: unsupported event %q", ch.ID, ev)
			}
		}
	}
	return nil
}

func ValidateNotifyProxies(cfg NotifyConfig, proxies []ProxyConfig) error {
	known := make(map[string]struct{}, len(proxies))
	for _, p := range proxies {
		known[p.ID] = struct{}{}
	}
	checkPair := func(label, primary, fallback string) error {
		if primary != "" && primary != ProxyFallbackNone && primary != ProxyFallbackDirect {
			if _, ok := known[primary]; !ok {
				return fmt.Errorf("%s 引用的代理 %q 不存在", label, primary)
			}
		}
		fb := NormalizeProxyFallback(fallback)
		if fb != ProxyFallbackNone && fb != ProxyFallbackDirect {
			if fb == primary {
				return fmt.Errorf("%s 的备用代理不能与主代理相同", label)
			}
			if _, ok := known[fb]; !ok {
				return fmt.Errorf("%s 引用的备用代理 %q 不存在", label, fb)
			}
		}
		return nil
	}

	if err := checkPair("Telegram Bot", cfg.TelegramBot.ProxyID, cfg.TelegramBot.FallbackProxyID); err != nil {
		return err
	}
	for _, ch := range cfg.Channels {
		label := ch.Name
		if label == "" {
			label = ch.ID
		}
		if err := checkPair("通知渠道 "+label, ch.ProxyID, ch.FallbackProxyID); err != nil {
			return err
		}
	}
	return nil
}

func normalizeNotifyProxyID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, ProxyFallbackNone) || strings.EqualFold(trimmed, ProxyFallbackDirect) {
		return ProxyFallbackNone
	}
	return trimmed
}

func normalizeNotifyEvents(events []string) []string {
	seen := make(map[string]struct{}, len(events))
	out := make([]string, 0, len(events))
	for _, item := range events {
		ev := strings.TrimSpace(strings.ToLower(item))
		if ev == "" {
			continue
		}
		if _, ok := seen[ev]; ok {
			continue
		}
		seen[ev] = struct{}{}
		out = append(out, ev)
	}
	return out
}

func normalizeStringList(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func parseCSVStrings(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	return normalizeStringList(parts)
}

func parseCSVInt64(raw string) ([]int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	var out []int64
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		val, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", trimmed)
		}
		out = append(out, val)
	}
	return dedupeInt64Slice(out), nil
}

func dedupeInt64Slice(items []int64) []int64 {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(items))
	out := make([]int64, 0, len(items))
	for _, id := range items {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func joinInt64CSV(items []int64) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, id := range items {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func escapeNotifyLineValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func unescapeNotifyLineValue(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 'r':
				b.WriteByte('\r')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
