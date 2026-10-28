# W-02：macOS arm64 鎖定 Go 工具鏈探查

此紀錄對應 REQ-060 與 T-119 的局部準備，不代表 Phego 引擎、PHP 相容性或產品驗收通過。2026-10-29 在 macOS 27.0.1（build 26A434）、Apple Silicon arm64，以鎖定的 Go 1.27.3 預編譯封存檔執行；不安裝 PHP 或工具鏈。

封存檔來自 [Go 下載站](https://go.dev/dl/go1.27.3.darwin-arm64.tar.gz)，與 `compat.lock.json` 中的 SHA-256 `dd3908cf20959d798c1e06ebe5b72213f0505ce01373b40ce3110975a6cd99c2` 相符。解壓與編譯均使用新建的暫存目錄；執行時 `sandbox-exec` 拒絕網路及該目錄以外的寫入，並以乾淨環境設定 `CGO_ENABLED=0`、`GOPROXY=off`、`GOTOOLCHAIN=local`。另以 Python 測試連線與目錄外建立暫存檔，兩者都回傳 `PermissionError`、errno 1。Python 啟動時曾輸出無法寫入系統 xcrun 快取的提示，測試本身回傳 0。

相同程式以兩個獨立建置快取編譯，兩次輸出均為 `probe go1.27.3 darwin/arm64`，兩個執行檔 SHA-256 均為 `0e7cba8aef1192e74b3cbad9738a9e2a0b0394ef20fcc25f78f43edf0e3b8554`，`cmp` 回傳 0。程式 SHA-256 為 `b9eceef2a7b64727d3ef5befb22d19bc763ea629ddd4c06a447a57de5952fe44`。重跑文件指令時，另一個暫存目錄內的兩個產物摘要一致，均為 `34b5e317f62df9047e5489af9cbda751487467a12c2c8a14b78a6a0b6e5ac3fa`，但與第一次不同。另以第一次解壓的同一工具鏈及第三個暫存目錄編譯，摘要為 `366c503d1b4aa4906f25466f1fde1b53d5ed883ebb8614ffc5dce54aef0996be`。目前尚未釐清跨目錄摘要不同的原因，因此不宣稱跨目錄、跨主機或從來源建置可重現。

## 重跑方式

在 macOS arm64 執行以下命令；封存檔從網路下載，版本及摘要須與來源鎖一致。暫存目錄的清理只作用於本次 `mktemp` 建立的目錄。

```sh
set -eu
probe_dir=$(mktemp -d /tmp/phego-darwin-go.XXXXXX)
probe_dir=$(cd "$probe_dir" && pwd -P)
trap 'rm -rf "$probe_dir"' EXIT
curl -fL --connect-timeout 15 --max-time 180 -o "$probe_dir/go.tar.gz" https://go.dev/dl/go1.27.3.darwin-arm64.tar.gz
test "$(shasum -a 256 "$probe_dir/go.tar.gz" | cut -d ' ' -f 1)" = dd3908cf20959d798c1e06ebe5b72213f0505ce01373b40ce3110975a6cd99c2
tar -xzf "$probe_dir/go.tar.gz" -C "$probe_dir"
mkdir "$probe_dir/src" "$probe_dir/cache1" "$probe_dir/cache2" "$probe_dir/home" "$probe_dir/tmp"
cat > "$probe_dir/src/main.go" <<'GO'
package main
import (
 "fmt"
 "runtime"
)
func main() { fmt.Printf("probe %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH) }
GO
cat > "$probe_dir/restrict.sb" <<'SB'
(version 1)
(allow default)
(deny network*)
(deny file-write*)
(allow file-write* (subpath (param "PROBE_DIR")) (literal "/dev/null"))
SB
for run_number in 1 2; do
  env -i PATH=/usr/bin:/bin HOME="$probe_dir/home" TMPDIR="$probe_dir/tmp" GOCACHE="$probe_dir/cache$run_number" GOTOOLCHAIN=local GOPROXY=off GO111MODULE=off CGO_ENABLED=0 sandbox-exec -f "$probe_dir/restrict.sb" -D PROBE_DIR="$probe_dir" "$probe_dir/go/bin/go" version
  env -i PATH=/usr/bin:/bin HOME="$probe_dir/home" TMPDIR="$probe_dir/tmp" GOCACHE="$probe_dir/cache$run_number" GOTOOLCHAIN=local GOPROXY=off GO111MODULE=off CGO_ENABLED=0 sandbox-exec -f "$probe_dir/restrict.sb" -D PROBE_DIR="$probe_dir" "$probe_dir/go/bin/go" build -trimpath -buildvcs=false -o "$probe_dir/probe$run_number" "$probe_dir/src/main.go"
  env -i PATH=/usr/bin:/bin HOME="$probe_dir/home" TMPDIR="$probe_dir/tmp" sandbox-exec -f "$probe_dir/restrict.sb" -D PROBE_DIR="$probe_dir" "$probe_dir/probe$run_number"
  shasum -a 256 "$probe_dir/probe$run_number"
done
cmp "$probe_dir/probe1" "$probe_dir/probe2"
```

此探查使用系統提供的 `sandbox-exec`；隔離控制僅以本次網路連線與目錄外寫入兩項測試確認，不宣稱涵蓋所有逃逸途徑。macOS amd64、Windows、Go 來源建置與產品本身仍未在此探查驗證。
