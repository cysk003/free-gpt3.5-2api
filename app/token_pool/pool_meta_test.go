package token_pool

import "testing"

func TestFindByTokenVariants(t *testing.T) {
	pool := GetAccessTokenPool()
	pool.Reset()
	pool.AddAccessToken(&AccessToken{
		Token:      "Bearer abc.def",
		PUID:       "p1",
		TeamUserID: "t1",
		ExpiresAt:  1 << 62,
	})
	for _, key := range []string{"Bearer abc.def", "abc.def"} {
		got := pool.FindByToken(key)
		if got == nil || got.PUID != "p1" || got.TeamUserID != "t1" {
			t.Fatalf("lookup %q failed: %+v", key, got)
		}
	}
}
