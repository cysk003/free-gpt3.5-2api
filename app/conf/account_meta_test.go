package conf

import (
	"testing"
	"time"

	"chat2api/app/token_pool"
)

func TestResolveTeamAndPUID(t *testing.T) {
	a := chatgpt{AccountId: "acc-1", TeamUserID: "team-2", PUID: " puid-3 "}
	if a.ResolveTeamUserID() != "team-2" {
		t.Fatalf("team priority failed: %q", a.ResolveTeamUserID())
	}
	if a.ResolvePUID() != "puid-3" {
		t.Fatalf("puid trim failed: %q", a.ResolvePUID())
	}
	b := chatgpt{AccountId: "acc-1"}
	if b.ResolveTeamUserID() != "acc-1" {
		t.Fatalf("account_id fallback failed: %q", b.ResolveTeamUserID())
	}
}

func TestNormalizeConfigFillsPoolMeta(t *testing.T) {
	cfg := app{
		ChatGPTs: []chatgpt{{
			AccessToken: "Bearer tok-meta-1",
			AccountId:   "team-abc",
			PUID:        "puid-xyz",
			Proxy:       "http://127.0.0.1:9",
		}},
	}
	normalizeConfig(&cfg)
	pool := token_pool.GetAccessTokenPool()
	if pool.Size() == 0 {
		t.Fatal("pool empty")
	}
	item := pool.FindByToken("Bearer tok-meta-1")
	if item == nil {
		t.Fatal("token not found")
	}
	if item.TeamUserID != "team-abc" {
		t.Fatalf("team=%q", item.TeamUserID)
	}
	if item.PUID != "puid-xyz" {
		t.Fatalf("puid=%q", item.PUID)
	}
	if item.Proxy != "http://127.0.0.1:9" {
		t.Fatalf("proxy=%q", item.Proxy)
	}
	// expire should be future
	if item.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("expires_at not future: %d", item.ExpiresAt)
	}
}
