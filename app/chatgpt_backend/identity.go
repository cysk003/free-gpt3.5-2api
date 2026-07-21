package chatgpt_backend

import (
	"strings"
	"sync"

	"github.com/google/uuid"
)

type sessionIdentity struct {
	DeviceID  string
	SessionID string
}

var (
	identityMu    sync.Mutex
	identityByKey = map[string]sessionIdentity{}
)

func resolveIdentity(authToken string) (deviceID, sessionID string) {
	key := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(authToken), "Bearer "))
	if key == "" {
		// noauth：device 即身份，session 独立生成。
		return uuid.NewString(), uuid.NewString()
	}
	identityMu.Lock()
	defer identityMu.Unlock()
	if cached, ok := identityByKey[key]; ok {
		return cached.DeviceID, cached.SessionID
	}
	cached := sessionIdentity{
		DeviceID:  uuid.NewString(),
		SessionID: uuid.NewString(),
	}
	identityByKey[key] = cached
	return cached.DeviceID, cached.SessionID
}

// RotateNoAuthIdentity 在 force-login/unauthorized 时轮换 noauth device。
func RotateNoAuthIdentity(c *Client) {
	if c == nil || c.AccAuth != "" {
		return
	}
	c.DeviceID = uuid.NewString()
	c.SessionID = uuid.NewString()
}
