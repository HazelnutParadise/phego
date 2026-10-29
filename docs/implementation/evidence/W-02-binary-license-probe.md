# W-02：Go 與 LLVM 二進位封存檔的部分授權文件清點

對應 REQ-060、T-119、T-120 的準備工作。每份封存檔依 `compat.lock.json` 的 component、版本、平台與 SHA-256 選取，先複製到容器私有暫存區並核對摘要，再清點檔名命中候選。結果為 `partial`，不代表授權適用性或散布義務已審查。

[七份整理結果](W-02-binary-license-files.json)的 SHA-256 為 `7a1ca7f1f8087f78085a92ac9094d04b76aa25ccd362eaad97f6ac96ee4a1980`。2026-10-29 每份獨立執行兩次，兩次 stdout 逐位元一致；[兩次原始輸出](W-02-binary-license-20261029/)均已保存。以下摘要是各份 stdout 原始位元組的 SHA-256。各次退出碼、隔離設定及清理狀態來自當次操作紀錄。

| 元件 | 平台 | 成員數 | 展開檔案內容 bytes | 候選數 | 單份輸出 SHA-256 |
|---|---|---:|---:|---:|---|
| go | darwin/amd64 | 17373 | 246149822 | 41 | `5b3d3a5def0e202c30827209718c630ee6fc1c8cb53f95a0d23719bc54392fe6` |
| go | darwin/arm64 | 17373 | 240004670 | 41 | `3c989c955eaac421b33f3d436592a63c1c9ea74ad1cb3251cc76c1b014b8b3eb` |
| go | linux/amd64 | 17373 | 244139206 | 41 | `4e2ab33644ef4aae8f10e0ccd8551bcc6dc74c975c5490e578e79fb6be891699` |
| go | linux/arm64 | 17373 | 238670456 | 41 | `0fc2a7ba1b527ed1e4c199eebc5ece4a9450032ada6c2cb01cbb5fcf766aac2e` |
| go | windows/amd64 | 15657 | 247160222 | 41 | `16ab048501099fefd46df057cdda287a42bcd53233a737f443f7d51d23d8aa73` |
| llvm | linux/amd64 | 11465 | 12407722424 | 1 | `8e057659021fbfdd31ea73ec6a660192681c2be1ca4b6eb94f87a4d4f7712e2d` |
| llvm | linux/arm64 | 11449 | 12032692409 | 1 | `1cd632b566793b33f02fc6bff2852c2f08bf423ce97fc02b5d5fd041238dbec6` |

## 執行環境與方式

清點時 `tools/archive_license_inventory.py` 的 SHA-256 為 `e1801d9566b6d5ea57d5967a15516cf30f17903dace4a27b778bfb7a5e4b30a2`，其引用的 `tools/upstream_snapshot.py` 為 `f5e462dff260892461dd1f94964eb1965f62e817d0eb7a488ce2d4d83c12ecb6`，`compat.lock.json` 為 `b2bd0913b1aa8bf485bae29b59247e203769ab170a4c2286086bee1d0619986e`。

使用 OrbStack 的 Linux arm64 容器，映像固定為 `python@sha256:29434a59c93070e470f84663e4289026e04644ea44b5220ef7e07d653318ca1f`，Python 3.14.10。容器不連網、以非 root 執行、根檔案系統唯讀、移除 capabilities，並限制為 2 CPU、3 GiB 記憶體、64 個程序、2 GiB 暫存空間與 600 秒執行時間。輸入掛載唯讀；可執行檔未在主機安裝或執行。

在已取得固定映像與來源封存檔的前提下，從專案根目錄以 Go Linux arm64 為例：

```sh
REPO="$PWD"
ARCHIVE=/absolute/path/to/go1.27.3.linux-arm64.tar.gz
docker run --rm --pull never --platform linux/arm64 --network none \
  --cpus 2 --memory 3g --pids-limit 64 --cap-drop ALL \
  --security-opt no-new-privileges --read-only --user 65534:65534 \
  --tmpfs /tmp:rw,noexec,nosuid,size=2g \
  --mount "type=bind,src=$REPO/tools,dst=/tool,readonly" \
  --mount "type=bind,src=$REPO/compat.lock.json,dst=/input/lock.json,readonly" \
  --mount "type=bind,src=$ARCHIVE,dst=/input/archive,readonly" \
  python@sha256:29434a59c93070e470f84663e4289026e04644ea44b5220ef7e07d653318ca1f \
  timeout 600 python3 /tool/archive_license_inventory.py capture \
  --lock /input/lock.json --component go --platform linux/arm64 --archive /input/archive
```

其他平台替換來源封存檔與 `--component`／`--platform`；LLVM 使用 `llvm`。工具驗證來源鎖後才解析副本，不下載任何來源。容器映像拉取不包含在上述 600 秒限制內。

## 限制

候選規則只比對檔名是否包含 LICENSE、LICENCE、COPYING、NOTICE 或 PATENTS，可能有漏檢或誤命中。清點不解開巢狀封存檔，也不判定檔案對實際散布是否適用。檔案內容摘要不是授權審查。映像內其他依賴的授權、來源標頭、歸屬與散布義務仍待確認；W-02 與產品驗收沒有因此通過。
