package chatgpt_backend

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"chat2api/app/browserfp"
	"chat2api/app/fingerprint"
)

const (
	defaultPowScript  = "https://chatgpt.com/backend-api/sentinel/sdk.js"
	sentinelSDKScript = "https://chatgpt.com/sentinel/20260423af3c/sdk.js"
	powMaxAttempts    = 500000
	powFailPrefix     = "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D"
)

var (
	scriptSrcRE = regexp.MustCompile(`<script\b[^>]*\bsrc=["']([^"']+)["']`)
	dataBuildRE = regexp.MustCompile(`(?:c/[^/]*/_|<html[^>]*data-build=["']([^"']*)["'])`)
)

type ProofWork struct {
	Difficulty string `json:"difficulty,omitempty"`
	Required   bool   `json:"required"`
	Seed       string `json:"seed,omitempty"`
	Ospt       string `json:"-"`
}

type Resources struct {
	ScriptSources []string
	DataBuild     string
}

func ParseResources(html string) Resources {
	resources := Resources{}
	for _, match := range scriptSrcRE.FindAllStringSubmatch(html, -1) {
		resources.ScriptSources = append(resources.ScriptSources, match[1])
		if resources.DataBuild == "" {
			if parts := strings.Split(match[1], "/"); len(parts) > 2 && parts[1] == "cdn" {
				// keep scanning
			}
		}
	}
	if m := dataBuildRE.FindStringSubmatch(html); len(m) > 1 && m[1] != "" {
		resources.DataBuild = m[1]
	}
	// fallback: try data-build attribute patterns already covered
	if resources.DataBuild == "" {
		if idx := strings.Index(html, `data-build="`); idx >= 0 {
			rest := html[idx+len(`data-build="`):]
			if end := strings.Index(rest, `"`); end > 0 {
				resources.DataBuild = rest[:end]
			}
		}
	}
	return resources
}

func CalcProofToken(seed string, difficulty string, userAgent string, deviceID string, resources ...Resources) string {
	if seed == "" || difficulty == "" {
		return "gAAAAAB~S"
	}
	start := time.Now()
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	res := firstResource(resources)
	for i := 0; i < powMaxAttempts; i++ {
		elapsed := time.Since(start).Milliseconds()
		config := buildFingerprintConfig(userAgent, deviceID, res, rng, &i, &elapsed)
		encoded := encodeConfig(config)
		hashResult := fnv1aHash(seed + encoded)
		if len(difficulty) > len(hashResult) {
			continue
		}
		if hashResult[:len(difficulty)] <= difficulty {
			return "gAAAAAB" + encoded + "~S"
		}
	}
	return "gAAAAAB" + powFailPrefix + base64.StdEncoding.EncodeToString([]byte(`"e"`)) + "~S"
}

func LegacyRequirementsToken(userAgent string, deviceID string, resources ...Resources) string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	config := buildFingerprintConfig(userAgent, deviceID, firstResource(resources), rng, nil, nil)
	return "gAAAAAC" + encodeConfig(config) + "~S"
}

// RequirementsTokenNonce2 对齐 /sentinel/req 用的 fingerprint token (nonce=2)。
func RequirementsTokenNonce2(userAgent string, deviceID string, resources ...Resources) string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonce := 2
	config := buildFingerprintConfig(userAgent, deviceID, firstResource(resources), rng, &nonce, nil)
	return "gAAAAAC" + encodeConfig(config) + "~S"
}

func firstResource(resources []Resources) Resources {
	if len(resources) > 0 {
		return resources[0]
	}
	return Resources{ScriptSources: []string{defaultPowScript}}
}

func buildFingerprintConfig(userAgent, deviceID string, resources Resources, rng *rand.Rand, nonce *int, elapsedMs *int64) []interface{} {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	fp := browserfp.Get()
	if fp == nil {
		browserfp.Init()
		fp = browserfp.Get()
	}
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		ua = browserfp.UserAgents[0]
	}
	buildID := fp.BuildID
	if strings.TrimSpace(resources.DataBuild) != "" {
		buildID = strings.TrimSpace(resources.DataBuild)
	}
	if buildID == "" {
		buildID = browserfp.DefaultBuildID
	}

	opts := fingerprint.Options{
		UserAgent:           ua,
		Languages:           []string{"en-US", "en"},
		Platform:            "Win32",
		ScreenWidth:         fp.ScreenWidth,
		ScreenHeight:        fp.ScreenHeight,
		HardwareConcurrency: fp.HardwareConcurrency,
		JSHeapSizeLimit:     fp.JSHeapSizeLimit,
		BuildID:             buildID,
		Timezone:            "America/Los_Angeles",
		PageOpenedSeconds:   float64(8 + rng.Intn(20)),
		Rand:                rng,
	}
	// 若页面资源里有 script，优先覆盖 ScriptURLs 随机池的 [5]
	config := fingerprint.Build25(opts)
	if len(resources.ScriptSources) > 0 {
		scripts := append([]string{}, resources.ScriptSources...)
		scripts = append(scripts, sentinelSDKScript)
		config[5] = scripts[rng.Intn(len(scripts))]
	}

	nonceValue := 1
	if nonce != nil {
		nonceValue = *nonce
	}
	config[3] = nonceValue

	if elapsedMs != nil {
		config[9] = float64(*elapsedMs)
	}

	if strings.TrimSpace(deviceID) == "" {
		deviceID = "00000000-0000-4000-8000-000000000000"
	}
	config[14] = deviceID
	return config
}

func encodeConfig(config []interface{}) string {
	data, err := json.Marshal(config)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

func fnv1aHash(text string) string {
	const (
		fnvOffset = 2166136261
		fnvPrime  = 16777619
	)
	h := uint32(fnvOffset)
	for _, ch := range text {
		h ^= uint32(ch)
		h = imul32(h, fnvPrime)
	}
	h ^= h >> 16
	h = imul32(h, 2246822507)
	h ^= h >> 13
	h = imul32(h, 3266489909)
	h ^= h >> 16
	return fmt.Sprintf("%08x", h)
}

func imul32(a, b uint32) uint32 {
	return uint32(int32(a) * int32(b))
}
