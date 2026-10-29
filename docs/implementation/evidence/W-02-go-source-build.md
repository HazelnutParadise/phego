# W-02｜Go 來源建置探查

此紀錄對應 REQ-060、T-119 與 [W-02](../W-02.md)。這是參考工具鏈的局部探查，不代表產品驗收；T-119 仍為 `not_run`。

2026-10-29 取得 [Go 1.27.3 官方來源封存檔](https://go.dev/dl/go1.27.3.src.tar.gz)，依 [Go 官方發布資料](https://go.dev/dl/?mode=json&include=all)核對 SHA-256 為 `6e7749035cf360f06b2575d162ea79df9fda8ef671889e5c375f8e50777f26bb`。以既有來源鎖的 Linux arm64 預編譯 Go 1.27.3 作為 bootstrap，封存檔 SHA-256 為 `a06e99309dbcbdca1a8ccd2282c3ee146e55930886e26b4f1193269587165f49`；本結果仍依賴該 bootstrap 與固定容器映像的可信度。

在 Mac 的 OrbStack Linux arm64 虛擬機，使用 Docker Server 29.8.2 與核心 `7.2.12-orbstack-00394-gbecdd6130068`，依下列命令在兩個新容器各執行一次。`SOURCE` 與 `BOOTSTRAP` 為上述已下載封存檔的絕對路徑，`CIDFILE` 應指向尚不存在的檔案：

```sh
docker run --rm --cidfile "$CIDFILE" --pull never --platform linux/arm64 \
  --network none --read-only --user 65534:65534 --pids-limit 256 \
  --memory 3g --cpus 2 --cap-drop ALL --security-opt no-new-privileges \
  --tmpfs /tmp:rw,nosuid,nodev,exec,size=2g \
  --mount "type=bind,src=$SOURCE,dst=/input/source.tar.gz,readonly" \
  --mount "type=bind,src=$BOOTSTRAP,dst=/input/bootstrap.tar.gz,readonly" \
  buildpack-deps@sha256:2d7fd9aab90c5444e6c4f5c59fce9ab6ba6bce7b8709ef74fcd1f618cd2e867b \
  timeout --kill-after=5s 1200s sh -ec '
    cp /input/source.tar.gz /tmp/source.tar.gz; cp /input/bootstrap.tar.gz /tmp/bootstrap.tar.gz
    printf "6e7749035cf360f06b2575d162ea79df9fda8ef671889e5c375f8e50777f26bb  /tmp/source.tar.gz\na06e99309dbcbdca1a8ccd2282c3ee146e55930886e26b4f1193269587165f49  /tmp/bootstrap.tar.gz\n" | sha256sum -c -
    mkdir -p /tmp/source /tmp/bootstrap /tmp/home /tmp/cache
    tar -xzf /tmp/source.tar.gz -C /tmp/source; tar -xzf /tmp/bootstrap.tar.gz -C /tmp/bootstrap
    export HOME=/tmp/home GOCACHE=/tmp/cache GOROOT_BOOTSTRAP=/tmp/bootstrap/go CGO_ENABLED=0 GOPROXY=off GOTOOLCHAIN=local
    /tmp/bootstrap/go/bin/go version; cd /tmp/source/go/src; ./make.bash
    /tmp/source/go/bin/go version; printf "package main\nfunc main() {}\n" > /tmp/probe.go
    cd /tmp; /tmp/source/go/bin/go build -o /tmp/probe /tmp/probe.go; /tmp/probe
    sha256sum /tmp/source/go/bin/go /tmp/probe; echo GO_SOURCE_BUILD_OK'
```

兩次命令均回傳 0，產生的 `go` 版本為 `go1.27.3 linux/arm64`，其執行檔 SHA-256 均為 `dac5084fdb1df984738a1c95ee1109c330389183aa1c51199c3b7bf1bf24c3ab`。兩次使用新建的 Go 編譯並執行最小程式，其摘要均為 `c08e2094e281475333e7bba80be3c97ec23bee166e2a0d60d5c49117bc23f22f`。[第一次輸出](W-02-go-source-20261029/linux-arm64-1.log)及[第二次輸出](W-02-go-source-20261029/linux-arm64-2.log)的 SHA-256 均為 `b08f752f3e0f97239d1c162ce5200f9a9d502335385a3429f5d903025078b349`。

兩次容器 ID 分別為 `3fa2e72e2479dd2f65f2d812f492966d8dd13e2956cbd448cbb39be95dcce9cf` 和 `f0f716736222b74c8af4d95c096aa5fe6f0a76606e50b72d4ce6edc9dd94b846`；後續 `docker inspect` 各回傳 1 和 `no such object`。結束碼與 inspect 結果未包含在上述原始輸出中。

上述兩次手動探查只涵蓋同一 Linux arm64 環境、單一來源與 bootstrap；只比對 `go` 執行檔和最小程式，沒有逐一比對整套工具鏈，也沒有執行完整 Go 測試或其他平台測試。官方來源與 bootstrap 的授權及供應鏈審查仍須完成。

## 可重跑工具的探查

[Go 參考工具](../../../tools/reference_go.py)新增 `--source-build`，以位置參數提供鎖定的預編譯 bootstrap，並依來源鎖驗證來源封存檔。2026-10-29 使用工具 SHA-256 `0078d5cc2691a9e278f62066e194c062e67b81d07c36c0469479c23833642f87`、來源鎖 SHA-256 `56676446ac1619a418f62ed56cfe975ff5a6af8f3dc547b8ca4366aaf1240c58`，在相同 OrbStack Linux arm64 環境執行下列命令兩次：

```sh
python3 tools/reference_go.py "$BOOTSTRAP" --source-build "$SOURCE" --platform linux/arm64 --timeout 1200
```

兩次均回傳 0，重建的 `go` 與最小程式摘要都分別與上段相同。完整輸出保存在[第一次](W-02-go-source-20261029/cli-arm64-1.log)與[第二次](W-02-go-source-20261029/cli-arm64-2.log)，SHA-256 依序為 `fa00265db67c945d62cf962b15fba05dbe250dc91abb12592582905096a2f031`、`8471daeb7345e13463cb300a38a5047eb9829c443cbfa74e5837188633c2eac3`。容器 ID 依序為 `a72c9a0c490c0204baae51ace2025b33acdbb68063a2c88974760f2ca9dae768`、`926072772d40040b34acef650872bd7f323b8dcb0c4798ce791ea410f9521254`；各自 `docker inspect` 均回傳 1 和 `no such object`。結束碼與 inspect 結果未包含在輸出檔中。

同一工具版本以 `--timeout 1` 執行回傳 124；容器 `daea2728f121b86b7a2d4fc771eecc077173e44571a21b34e8ab0aacf0e06e38` 的 inspect 回傳 1 和 `no such object`。另一個容器 `8b89ff9e5f2348fde2ba39999631cec39303853e0471fda90dd1fde9598a10fa` 經 inspect 確認正在執行後，對工具送 SIGTERM，工具回傳 143，後續 inspect 回傳 1 和 `no such object`。這些探查不代表所有故障已驗證。

`python3 -m unittest discover -s tools -p test_reference_go_source.py` 執行 13 項通過，包含命令輸入與隔離條件，以及以受控替身執行 shell 的來源、bootstrap、建置失敗與產物檢查；替身測試不代表實際跨平台相容。

## Linux amd64 轉譯環境

以同一工具與來源鎖版本，在同一 OrbStack Linux 虛擬機的 amd64 轉譯環境，使用已核對摘要 `65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40` 的 Linux amd64 Go 1.27.3 預編譯封存檔作 bootstrap，執行兩次：

```sh
python3 tools/reference_go.py "$BOOTSTRAP_AMD64" --source-build "$SOURCE" --platform linux/amd64 --timeout 1800
```

兩次回傳 0，重建的 `go` 版本均為 `go1.27.3 linux/amd64`，執行檔 SHA-256 均為 `fea858cf420141c20f0ac45b84595e416ee66b14d8c308068037ec771a0bc236`；各自編譯並執行的最小程式 SHA-256 均為 `850c3000c369daed039ec0e0e217f38a09bdb11e1146675be42488e153647410`。完整輸出保存在[第一次](W-02-go-source-20261029/cli-amd64-1.log)與[第二次](W-02-go-source-20261029/cli-amd64-2.log)，SHA-256 依序為 `28fd9df3aa343da9f735ad8e53446045a57933b4f4a05f0957f0742d5c01cce7`、`d50dba1429cb1e9c6a164500dcc5eff8db308902e80a46349881d1698bc5cc95`。容器 ID 依序為 `6c2e82be67978cf641534aa77efb549cd589a4313d3be801d6d5d854bbb420ae`、`f237e90b78628c1ef80d4099b489fc564af1d998c6012e81c5738947dbc92407`；各自 `docker inspect` 均回傳 1 和 `no such object`。結束碼與 inspect 結果未包含在輸出檔中。

結果僅支持此轉譯環境、來源、bootstrap、映像與設定下對 `go` 執行檔和最小程式的比較；原生 amd64、其他環境、整套工具鏈與完整 Go 測試仍未驗證。
