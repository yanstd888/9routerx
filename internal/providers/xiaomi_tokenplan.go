// 9RouterX — all source code port and modification by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers

import "strings"

// Xiaomi Token Plan is one product served from three regional origins. The
// registry names only one of them (sgp), and the dashboard's connection probe
// already knows the full map — but the chat lane did not, so a connection
// created with region=cn was routed to the Singapore host and answered
// "Invalid API Key" against a perfectly valid key.
//
// The regions are declared here so both lanes read the same table; the
// dashboard probe aliases these entries rather than keeping a second copy.

// XiaomiTokenplanRegionHosts maps a Token Plan region to its API root (the
// part before /chat/completions).
var XiaomiTokenplanRegionHosts = map[string]string{
	"sgp": "https://token-plan-sgp.xiaomimimo.com/v1",
	"cn":  "https://token-plan-cn.xiaomimimo.com/v1",
	"ams": "https://token-plan-ams.xiaomimimo.com/v1",
}

// XiaomiTokenplanDefaultRegion is used when a connection stores no region, or
// stores one this build does not know. It matches the registry base URL, so an
// unconfigured connection keeps the behaviour it had before regions existed.
const XiaomiTokenplanDefaultRegion = "sgp"

// XiaomiTokenplanBaseURL resolves the API root for a stored region. An unknown
// or empty region falls back to the default rather than dialing a literal.
func XiaomiTokenplanBaseURL(region string) string {
	if host, ok := XiaomiTokenplanRegionHosts[strings.ToLower(strings.TrimSpace(region))]; ok {
		return host
	}
	return XiaomiTokenplanRegionHosts[XiaomiTokenplanDefaultRegion]
}

// XiaomiTokenplanRegionOf reads the region a connection stored. The field
// lives in providerSpecificData, which is a free-form map on the wire.
func XiaomiTokenplanRegionOf(psd map[string]any) string {
	if psd == nil {
		return ""
	}
	region, _ := psd["region"].(string)
	return region
}

// XiaomiTokenplanChatURL is the chat-completions route for a stored region.
func XiaomiTokenplanChatURL(region string) string {
	return XiaomiTokenplanBaseURL(region) + "/chat/completions"
}
