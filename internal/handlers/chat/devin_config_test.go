// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package chat

import (
	"testing"
	"9routerx/proxy/internal/db"
)

func TestDevinDesktopServiceOverrideIsNotOpenAIEndpoint(t *testing.T) {
	database,cleanup:=setupChatTestDB(t);defer cleanup()
	h:=NewChatHandler(db.NewRepo(database))
	for _,base:=range []string{"https://native.devin.example", "https://native.devin.example/", "https://native.devin.example/exa.api_server_pb.ApiServerService/GetChatMessage"} {
		cfg,err:=h.getProviderConfig("devin-desktop",&ConnectionData{BaseURL:base})
		if err!=nil {t.Fatal(err)}
		if cfg.BaseURL!="https://native.devin.example" && cfg.BaseURL!="https://native.devin.example/exa.api_server_pb.ApiServerService/GetChatMessage" {t.Fatalf("native service incorrectly normalized: %q",cfg.BaseURL)}
	}
}
