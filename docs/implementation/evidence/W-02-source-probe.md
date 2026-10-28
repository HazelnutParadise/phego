# W-02 參考來源與隔離建置探查

2026-10-28 從 [PHP 8.5 發布資料](https://www.php.net/releases/index.php?json&version=8.5)讀取 8.5.12 的 tar.xz SHA-256，並下載 [官方來源](https://www.php.net/distributions/php-8.5.12.tar.xz)。實際檔案摘要與官方值均為 `08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209`。官方 `php-src` 的 `php-8.5.12` tag 物件為 `c96586077b050b362744ba295f97f6395f58a75a`，指向提交 `136e049ac10b032c83018e65b63b6df858943750`。封存檔包含上游 `LICENSE`，其內容標示 PHP License 3.01。

## 隔離建置實測

Docker Server 29.8.2，Linux arm64；使用 `buildpack-deps@sha256:2d7fd9aab90c5444e6c4f5c59fce9ab6ba6bce7b8709ef74fcd1f618cd2e867b`。容器沒有網路，根檔案系統唯讀，來源封存檔唯讀掛載，以 UID/GID 65534 執行，移除所有 capabilities，限制 2 CPU、2 GiB 記憶體及 256 個程序。容器內 Debian 12.15 的 `cc` 是 12.2.0、GNU ld 是 2.40、GNU Make 是 4.3。

執行操作如下，`$ARCHIVE` 是已核對摘要的來源封存檔路徑：

```sh
docker run --rm --pull never --network none --read-only \
  --tmpfs /tmp:rw,nosuid,nodev,exec,size=2g --user 65534:65534 \
  --pids-limit 256 --memory 2g --cpus 2 --cap-drop ALL \
  --security-opt no-new-privileges \
  --mount "type=bind,src=$ARCHIVE,dst=/input/php.tar.xz,readonly" \
  buildpack-deps@sha256:2d7fd9aab90c5444e6c4f5c59fce9ab6ba6bce7b8709ef74fcd1f618cd2e867b \
  sh -ec 'cd /tmp; sha256sum /input/php.tar.xz; tar -xJf /input/php.tar.xz; cd php-8.5.12; ./configure --disable-all --enable-cli --without-pear > /tmp/configure.log 2>&1 || { tail -80 /tmp/configure.log; exit 1; }; make -j2 > /tmp/make.log 2>&1 || { tail -80 /tmp/make.log; exit 1; }; sapi/cli/php -n -v; sha256sum sapi/cli/php; sapi/cli/php -n run-tests.php -n -q tests/basic/001.phpt'
```

實際輸出為 PHP 8.5.12 CLI，二進位摘要 `57f8cddb4b93728d91543d5a021ac630ffefa8348c4dd396e5e661612ad093f7`，上游 `tests/basic/001.phpt` 1 項通過。此結果只驗證參考引擎的單次建置與測試，不代表 Phego 相容，也尚未證明兩次建置的產物一致。

另外以相同映像與選項，在兩個新容器加入 `SOURCE_DATE_EPOCH=1792540800`、`TZ=UTC`、`LC_ALL=C` 及容器內 900 秒期限各建置一次。兩次均通過同一項 PHPT，但二進位摘要分別為 `e71d1eec0e12247f94e2c08a094015cf54bc869d174395b141585c3347c83030` 與 `0712a4b6e67f709c1d5c1f5fa0a0ea51349f70d1ec510107c4bd0fc678740b75`。容器主機名稱不同是待驗證的可能原因，目前不可宣稱可重現。

在後續固定主機名稱為 `phego-reference` 的試驗中，產生的 `main/php_config.h` 內 `PHP_BUILD_SYSTEM` 與 `PHP_UNAME` 都包含容器主機名稱。使用相同映像、來源、平台、主機名稱與上述環境設定重建兩次，二進位 SHA-256 均為 `d562342982da42e1abfa98c4d5d7ffc903a5e448836cd9946bb403ea87b224af`，兩次各通過同一項 PHPT。此證據限於這兩次 Linux arm64 建置及當時的核心版本，不代表其他平台或環境可重現。

重跑時在上述 `docker run` 的映像名稱前加入 `--platform linux/arm64 --hostname phego-reference --env SOURCE_DATE_EPOCH=1792540800 --env TZ=UTC --env LC_ALL=C`，並將映像後的 `sh -ec` 改為 `timeout --kill-after=5s 900s sh -ec`。

首次試驗因 Docker 的 `/tmp` tmpfs 預設為 `noexec`，`configure` 回傳 permission denied；確認掛載選項後，在隔離容器的暫存目錄明確使用 `exec` 才成功。

## Go 工具鏈探查

從 [Go 官方下載資料](https://go.dev/dl/?mode=json)取得 Linux arm64 的 Go 1.27.3 摘要，實際下載的 `go1.27.3.linux-arm64.tar.gz` 摘要與官方值均為 `a06e99309dbcbdca1a8ccd2282c3ee146e55930886e26b4f1193269587165f49`。在同一受限容器映像中解開後，`go version` 回報 `go1.27.3 linux/arm64`，以 `CGO_ENABLED=0`、`GOPROXY=off` 編譯並執行最小 Go 程式成功。此探查不是 Phego 建置測試。

```sh
docker run --rm --pull never --network none --read-only \
  --tmpfs /tmp:rw,nosuid,nodev,exec,size=1g --user 65534:65534 \
  --pids-limit 128 --memory 1g --cpus 2 --cap-drop ALL \
  --security-opt no-new-privileges \
  --mount "type=bind,src=$GO_ARCHIVE,dst=/input/go.tar.gz,readonly" \
  buildpack-deps@sha256:2d7fd9aab90c5444e6c4f5c59fce9ab6ba6bce7b8709ef74fcd1f618cd2e867b \
  sh -ec 'sha256sum /input/go.tar.gz; cd /tmp; tar -xzf /input/go.tar.gz; /tmp/go/bin/go version; printf "package main\nfunc main() {}\n" > main.go; HOME=/tmp GOPATH=/tmp/gopath GOCACHE=/tmp/gocache CGO_ENABLED=0 GOPROXY=off /tmp/go/bin/go build -o /tmp/probe /tmp/main.go; /tmp/probe; printf "pure-Go build: PASS\n"'
```

## 可重跑的參考建置工具

`python3 tools/reference_php.py "$ARCHIVE"` 使用來源鎖核對摘要，並在固定主機名稱、路徑及環境的受限容器內建置與執行單一上游 PHPT。三次工具建置均得到二進位 SHA-256 `e5d0591e0d82e30f413e4aa928ff66afddd1f752c707b17feffaae70c4f8b519`，且各有一項 PHPT 通過；最後一次使用腳本 SHA-256 `725f90f1be0582064fe56d7af7936f2f9b5f3bc1f21d773b5d51729981b2feae`，回傳 0，並以 Docker inspect 確認容器已移除。這個工具使用的建置路徑不同於前述手動試驗，因此兩組摘要不可直接比較。

以目前腳本測試中斷：一秒容器期限回傳 124，SIGTERM 回傳 143，兩次皆以 Docker inspect 確認容器已移除。SIGKILL 無法由腳本處理，Docker 無法回應時清理狀態也可能未知。

LLVM、其他平台、上游更新偵測、跨環境重現性及完整驗收仍待完成。
