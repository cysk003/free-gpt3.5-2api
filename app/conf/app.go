package conf

import "strings"

type app struct {
	LogLevel       string    `yaml:"log_level"`
	LogPath        string    `yaml:"log_path"`
	LogFile        string    `yaml:"log_file"`
	Bind           string    `yaml:"bind"`
	Port           uint16    `yaml:"port"`
	Auth           auth      `yaml:"auth"`
	Proxy          string    `yaml:"proxy"`
	ChatGPTBaseUrl string    `yaml:"chatgpt_base_url"`
	ChatGPTs       []chatgpt `yaml:"chatgpts"`
}

func (a app) TextAccessTokens() []string {
	tokens := make([]string, 0, len(a.ChatGPTs))
	for _, account := range a.ChatGPTs {
		if account.AccessToken != "" {
			tokens = append(tokens, account.AccessToken)
		}
	}
	return tokens
}

type auth struct {
	AccessTokens      []string `yaml:"access_tokens"`
	AccessTokenPrefix []string `yaml:"access_token_prefix"`
}

func (a app) DirectAccessToken(localToken string) (string, bool) {
	token, matched := a.matchAccessTokenPrefix(localToken)
	return token, matched && token != ""
}

func (a app) DirectAccessTokenPrefixMatched(localToken string) bool {
	_, matched := a.matchAccessTokenPrefix(localToken)
	return matched
}

func (a app) HasAccessTokenPrefix() bool {
	return len(a.Auth.AccessTokenPrefix) > 0
}

func (a app) matchAccessTokenPrefix(localToken string) (string, bool) {
	localToken = strings.TrimSpace(localToken)
	for _, prefix := range a.Auth.AccessTokenPrefix {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" || !strings.HasPrefix(localToken, prefix) {
			continue
		}
		token := strings.TrimSpace(strings.TrimPrefix(localToken, prefix))
		return token, true
	}
	return "", false
}

type chatgpt struct {
	IdToken      string `yaml:"id_token"`
	AccessToken  string `yaml:"access_token"`
	RefreshToken string `yaml:"refresh_token"`
	// AccountId 映射为 Chatgpt-Account-Id（Team workspace）。
	AccountId string `yaml:"account_id"`
	// TeamUserID 可选别名；非空时优先于 account_id。
	TeamUserID  string `yaml:"team_user_id"`
	PUID        string `yaml:"puid"`
	LastRefresh string `yaml:"last_refresh"`
	Email       string `yaml:"email"`
	Type        string `yaml:"type"`
	Expired     string `yaml:"expired"`
	Proxy       string `yaml:"proxy"`
}

// ResolveTeamUserID 返回应注入 Chatgpt-Account-Id 的值。
func (a chatgpt) ResolveTeamUserID() string {
	if v := strings.TrimSpace(a.TeamUserID); v != "" {
		return v
	}
	return strings.TrimSpace(a.AccountId)
}

// ResolvePUID 返回应注入 _puid cookie 的值。
func (a chatgpt) ResolvePUID() string {
	return strings.TrimSpace(a.PUID)
}
