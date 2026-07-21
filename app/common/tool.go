package common

import (
	"chat2api/app/constant"

	"github.com/bogdanfinn/tls-client/profiles"
)

// FixedUserAgent 对齐 aurora/util.FixedUserAgent（桌面 Chrome 148）。
const FixedUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"

// ClientBuildNumber 对齐 aurora headerbuilder。
const ClientBuildNumber = "7823760"

var (
	clientProfile   = profiles.Chrome_124
	ua              = FixedUserAgent
	updateThreshold = constant.ReTry
)

func SubUpdateThreshold() {
	updateThreshold--
}

// GetClientProfile 固定桌面 Chrome TLS 画像，避免与 UA/sec-ch-ua 互相打架。
// 当前依赖链 tls-client@v1.7.5 最高可用 Chrome_124。
func GetClientProfile() profiles.ClientProfile {
	return clientProfile
}

// GetUa 返回固定桌面 Chrome UA。
func GetUa() string {
	return ua
}
