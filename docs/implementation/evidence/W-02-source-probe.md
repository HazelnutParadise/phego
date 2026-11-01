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

另外下載並核對官方下載資料列出的 Go 1.27.3 Linux amd64、macOS amd64／arm64、Windows amd64 封存檔；各摘要記錄於來源鎖的 `additional_archives`。Windows 第一次下載逾時，重新下載後摘要及檔案大小才符合官方資料。其中 Linux amd64 的 Go 1.27.3 已在 Mac 的 OrbStack Linux arm64 虛擬機中透過 amd64 容器轉譯執行，`CGO_ENABLED=0`、`GOPROXY=off` 編譯並執行最小 Go 程式成功，產物 SHA-256 為 `5696deac3649bad0068e7c2ca350ccb0e48dde0e3c09676530a3382982917e43`。macOS、Windows 與原生 amd64 主機仍未執行；此探查不是 Phego 建置測試。

Linux amd64 探查使用唯讀來源掛載與無網路的受限容器，映像是 `debian@sha256:89c1ea91e897f0fe0a3731595330bc45b8bdef4047464a07116f891a94054b31`。重跑時將 `$GO_ARCHIVE` 設為已下載的 Linux amd64 封存檔：

```sh
docker run --rm --pull never --platform linux/amd64 --network none --read-only \
  --tmpfs /tmp:rw,nosuid,nodev,exec,size=1g --user 65534:65534 \
  --pids-limit 128 --memory 1g --cpus 2 --cap-drop ALL --security-opt no-new-privileges \
  --mount "type=bind,source=$GO_ARCHIVE,target=/input/go.tar.gz,readonly" \
  debian@sha256:89c1ea91e897f0fe0a3731595330bc45b8bdef4047464a07116f891a94054b31 \
  timeout 180s sh -ec 'printf "65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40  /input/go.tar.gz\n" | sha256sum -c -; cd /tmp; tar -xzf /input/go.tar.gz; /tmp/go/bin/go version; printf "package main\nfunc main() {}\n" > main.go; HOME=/tmp GOPATH=/tmp/gopath GOCACHE=/tmp/gocache CGO_ENABLED=0 GOPROXY=off /tmp/go/bin/go build -o /tmp/probe /tmp/main.go; /tmp/probe; sha256sum /tmp/probe'
```

## 可選 LLVM 工具鏈探查

從 [LLVM 23.1.4 官方發布資料](https://github.com/llvm/llvm-project/releases/tag/llvmorg-23.1.4)下載 Linux arm64 的 `.tar.zst`；實際 SHA-256 與 GitHub 發行檔資料的摘要均為 `0975f753e0f9b5f83146a7a2f4678bc143e713496b56520716a4de047a63f99f`。tag 物件是 `f2c432f21b5e6f96157b1720857b472dc487fa49`，指向 `7a941dfb2877a0b8cf30323fefa85ad019e0b0f5`。上游授權為 Apache-2.0 WITH LLVM-exception。

以既有的 `zstd` 將已驗證封存檔解壓為標準輸入，僅在無網路、唯讀根目錄、非 root、移除 capabilities 的容器暫存空間擷取 clang 與 lld。映像固定為 `buildpack-deps@sha256:4c9dc8ac4ff58f6fe23d2b007fe3bb8e25716f947adae6fb312d6cfbaac2b3c3`（Debian 13、Linux arm64、glibc 2.41、GNU ld 2.44）；記憶體上限 2 GiB、2 CPU、128 程序及 180 秒期限。`clang --version` 和 `ld.lld --version` 均為 23.1.4，最小 C 程式經 clang、LLD 編譯連結後執行成功，二進位摘要 `055f61449e18ef4e6de750dae7bc89573acb4547f6823c8d92258239e7d2a9ce`。未安裝至宿主系統；未驗證 Go ABI 橋接、其他平台或 Phego 原生後端。

```sh
zstd -dc --long=30 "$LLVM_ARCHIVE" | docker run --rm -i --pull never --network none --read-only \
  --tmpfs /tmp:rw,nosuid,nodev,exec,size=1500m --user 65534:65534 \
  --pids-limit 128 --memory 2g --cpus 2 --cap-drop ALL \
  --security-opt no-new-privileges --platform linux/arm64 \
  buildpack-deps@sha256:4c9dc8ac4ff58f6fe23d2b007fe3bb8e25716f947adae6fb312d6cfbaac2b3c3 \
  timeout --kill-after=5s 180s sh -ec 'cd /tmp; tar -xf - LLVM-23.1.4-Linux-ARM64/bin/clang-23 LLVM-23.1.4-Linux-ARM64/bin/clang LLVM-23.1.4-Linux-ARM64/bin/lld LLVM-23.1.4-Linux-ARM64/bin/ld.lld; cd LLVM-23.1.4-Linux-ARM64; bin/clang --version; bin/ld.lld --version; printf "int main(void) { return 0; }\n" > /tmp/probe.c; bin/clang -fuse-ld=lld /tmp/probe.c -o /tmp/probe; /tmp/probe; sha256sum /tmp/probe; printf "clang and lld probe: PASS\n"'
```

## 可重跑的參考建置工具

`python3 tools/reference_php.py "$ARCHIVE"` 使用來源鎖核對摘要，並在固定主機名稱、路徑及環境的受限容器內建置與執行單一上游 PHPT。三次工具建置均得到二進位 SHA-256 `e5d0591e0d82e30f413e4aa928ff66afddd1f752c707b17feffaae70c4f8b519`，且各有一項 PHPT 通過；最後一次使用腳本 SHA-256 `725f90f1be0582064fe56d7af7936f2f9b5f3bc1f21d773b5d51729981b2feae`，回傳 0，並以 Docker inspect 確認容器已移除。這個工具使用的建置路徑不同於前述手動試驗，因此兩組摘要不可直接比較。

以目前腳本測試中斷：一秒容器期限回傳 124，SIGTERM 回傳 143，兩次皆以 Docker inspect 確認容器已移除。SIGKILL 無法由腳本處理，Docker 無法回應時清理狀態也可能未知。

LLVM、其他平台、上游更新偵測、跨環境重現性及完整驗收仍待完成。
