// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package media

// Blank-import the testnet seal: with ROUTERX_BLOCK_NET=1 (CI's test job) the
// package's tests boot with outbound network sealed, so an un-stubbed upstream
// call fails with "stub your upstream" instead of a mystery 403 from the real
// service. Without the env this import is inert; packages whose TestMain calls
// testnet.Seal() stay sealed either way (Seal is idempotent).
import _ "9routerx/proxy/internal/testnet"
