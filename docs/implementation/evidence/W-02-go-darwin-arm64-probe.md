# W-02：macOS arm64 鎖定 Go 工具鏈探查

此紀錄對應 REQ-060 與 T-119 的局部準備，不代表 Phego 引擎、PHP 相容性或產品驗收通過。2026-10-29 在 macOS 27.0.1（build 26A434）、Apple Silicon arm64，以鎖定的 Go 1.27.3 預編譯封存檔執行；不安裝 PHP 或工具鏈。

封存檔來自 [Go 下載站](https://go.dev/dl/go1.27.3.darwin-arm64.tar.gz)，與 `compat.lock.json` 中的 SHA-256 `dd3908cf20959d798c1e06ebe5b72213f0505ce01373b40ce3110975a6cd99c2` 相符。解壓與編譯均使用新建的暫存目錄；執行時 `sandbox-exec` 拒絕網路及該目錄以外的寫入，並以乾淨環境設定 `CGO_ENABLED=0`、`GOPROXY=off`、`GOTOOLCHAIN=local`。另以 Python 測試連線與目錄外建立暫存檔，兩者都回傳 `PermissionError`、errno 1。Python 啟動時曾輸出無法寫入系統 xcrun 快取的提示，測試本身回傳 0。

相同程式以兩個獨立建置快取編譯，兩次輸出均為 `probe go1.27.3 darwin/arm64`，兩個執行檔 SHA-256 均為 `0e7cba8aef1192e74b3cbad9738a9e2a0b0394ef20fcc25f78f43edf0e3b8554`，`cmp` 回傳 0。程式 SHA-256 為 `b9eceef2a7b64727d3ef5befb22d19bc763ea629ddd4c06a447a57de5952fe44`。重跑文件指令時，另一個暫存目錄內的兩個產物摘要一致，均為 `34b5e317f62df9047e5489af9cbda751487467a12c2c8a14b78a6a0b6e5ac3fa`，但與第一次不同。另以第一次解壓的同一工具鏈及第三個暫存目錄編譯，摘要為 `366c503d1b4aa4906f25466f1fde1b53d5ed883ebb8614ffc5dce54aef0996be`。差異來自原探查設定 `GO111MODULE=off`：Go 1.27.3 在這種模式下將來源目錄轉成 local prefix，納入建置識別。以 `GODEBUG=gocachehash=1` 檢查時，兩次記錄的 prefix 分別指向各自的來源目錄；同一工具鏈封存檔的 `src/cmd/go/internal/load/pkg.go:3340` 與 `src/cmd/go/internal/work/exec.go:310` 也顯示該值如何產生及加入識別。改用固定模組路徑後，兩個目錄各自解壓工具鏈、使用獨立快取編譯，兩個產物皆為 `52299c956a88294761d453e06484e2161afcd891d869cbb4c37707a1339e7059`，執行輸出相同且 `cmp` 回傳 0。這是同一台 Mac 上的最小 Go 程式探查，不代表跨主機或完整工具鏈從來源建置可重現。

## 重跑方式

在 macOS arm64 執行以下命令；封存檔從網路下載，版本及摘要須與來源鎖一致。暫存目錄的清理只作用於本次 `mktemp` 建立的目錄。

```sh
set -eu
probe_dir=$(mktemp -d /tmp/phego-darwin-go.XXXXXX)
probe_dir=$(cd "$probe_dir" && pwd -P)
trap 'rm -rf "$probe_dir"' EXIT
curl -fL --connect-timeout 15 --max-time 180 -o "$probe_dir/go.tar.gz" https://go.dev/dl/go1.27.3.darwin-arm64.tar.gz
test "$(shasum -a 256 "$probe_dir/go.tar.gz" | cut -d ' ' -f 1)" = dd3908cf20959d798c1e06ebe5b72213f0505ce01373b40ce3110975a6cd99c2
cat > "$probe_dir/restrict.sb" <<'SB'
(version 1)
(allow default)
(deny network*)
(deny file-write*)
(allow file-write* (subpath (param "PROBE_DIR")) (literal "/dev/null"))
SB
for run_number in 1 2; do
  run_dir="$probe_dir/run$run_number"
  mkdir -p "$run_dir/src" "$run_dir/cache" "$run_dir/home" "$run_dir/tmp"
  tar -xzf "$probe_dir/go.tar.gz" -C "$run_dir"
  printf 'module example.com/phego/repro-probe\n\ngo 1.27\n' > "$run_dir/src/go.mod"
  cat > "$run_dir/src/main.go" <<'GO'
package main
import (
 "fmt"
 "runtime"
)
func main() { fmt.Printf("probe %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH) }
GO
  (cd "$run_dir/src" && env -i PATH=/usr/bin:/bin HOME="$run_dir/home" TMPDIR="$run_dir/tmp" GOCACHE="$run_dir/cache" GOTOOLCHAIN=local GOPROXY=off GO111MODULE=on CGO_ENABLED=0 sandbox-exec -f "$probe_dir/restrict.sb" -D PROBE_DIR="$run_dir" "$run_dir/go/bin/go" build -trimpath -buildvcs=false -o "$run_dir/probe" .)
  env -i PATH=/usr/bin:/bin HOME="$run_dir/home" TMPDIR="$run_dir/tmp" sandbox-exec -f "$probe_dir/restrict.sb" -D PROBE_DIR="$run_dir" "$run_dir/probe"
  shasum -a 256 "$run_dir/probe"
done
cmp "$probe_dir/run1/probe" "$probe_dir/run2/probe"
```

此探查使用系統提供的 `sandbox-exec`；隔離控制僅以本次網路連線與目錄外寫入兩項測試確認，不宣稱涵蓋所有逃逸途徑。macOS amd64、Windows、Go 來源建置與產品本身仍未在此探查驗證。
