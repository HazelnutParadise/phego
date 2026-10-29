# W-02：LLVM Linux amd64 隔離探查

對應 REQ-060、T-119。此紀錄只證明下列工具鏈探查，不代表 Phego 的原生後端、Go ABI 或垃圾回收整合已完成。

## 來源與驗證

- LLVM 23.1.4 官方 [Linux X64 封存檔](https://github.com/llvm/llvm-project/releases/download/llvmorg-23.1.4/LLVM-23.1.4-Linux-X64.tar.zst)，大小 1,190,060,068 bytes，SHA-256 `efdd1dbd089b99bfcfcde7b8eb1236163f2e88df5a981cca4f783ec9cb5e0378`。
- 官方 [attestation bundle](https://github.com/llvm/llvm-project/releases/download/llvmorg-23.1.4/LLVM-23.1.4-Linux-X64.tar.zst.jsonl) 的 SHA-256 為 `90e6c020ac93350c7ab06bde3872fe4ce3b290431e8649c85dcfc8a0ff00f35d`。[留存 bundle](W-02-llvm-amd64-20261029/attestation-bundle.jsonl)。
- 使用 GitHub CLI 2.96.0 執行 `gh attestation verify --repo llvm/llvm-project ARCHIVE --bundle BUNDLE --format json`，回傳 0；CLI 版本與退出碼來自當次操作紀錄。[驗證結果](W-02-llvm-amd64-20261029/attestation-verification.json) SHA-256 為 `146cb17b4e8629300097615b42dfedbbb8fdb803f558525feea8a69217330a79`。

## 實測結果

在 Apple Silicon Mac 的 OrbStack 上以 amd64 轉譯執行，容器使用 Debian 13 的 `buildpack-deps@sha256:0766bfc10a5b87c895c6f675bb8dd09067821b4de144b047a6d029e0c366ecac`，關閉網路並移除 capabilities。這不是原生 amd64 硬體驗證。

三次獨立容器探查都成功執行 Clang 23.1.4 和 LLD 23.1.4，編譯、連結並執行 C 程式。產生的執行檔 SHA-256 均為 `e051785175b27ca3646f7f0e2049bd13f329d3cfc33deba49fdc13d633dce308`。三個容器事後均已移除；容器設定、獨立執行與清理狀態來自當次操作紀錄，未留在下列輸出中。

- [第一次完整輸出](W-02-llvm-amd64-20261029/probe-1.log.gz)：解壓後 SHA-256 `35ef1b8c7d6e74256dfeb54d1f3bdc672fd9e6216a3c97b291c73a9fee586d8f`，含 `readelf`；[封存檔目錄](W-02-llvm-amd64-20261029/layout.txt.gz) 解壓後 SHA-256 `e59532af297a23e95ecf7d23871eb973a5f20c8f9830d74e7bb9d8ee3d7ac56a`。
- [第二次輸出](W-02-llvm-amd64-20261029/probe-2.log)：SHA-256 `891f377c9273e737ad3f69911be10f5b36b7780e0da9cc596b6d76b4b58652ab`。
- [第三次輸出](W-02-llvm-amd64-20261029/probe-gated.log)：SHA-256 `12e0f547ce37470c605aa63c6c49a72fc1138e083c4a780ec8f99651d574b171`。同一封存檔資料流先計算 SHA-256，容器解壓後等待唯讀掛載目錄內的驗證訊號，確認相符才執行解壓的程式。[資料流雜湊](W-02-llvm-amd64-20261029/gated-stream.sha) 的檔案 SHA-256 為 `9c17435305045714d712154b92c8ede9d083afab543826ce5fe0c0ac1623245e`。
- [錯誤雜湊測試](W-02-llvm-amd64-20261029/gate-reject.log) 使用短測試輸入，回傳 1 並記錄 `stream_digest_mismatch`，當次操作紀錄未見受保護的程式執行。這只測試驗證閘門，不代表完整 LLVM 解壓失敗路徑已測試。

前兩次是在執行後確認輸入資料流雜湊；只有第三次在執行前等待雜湊驗證。資料流和訊號由同一位主機使用者控制，此閘門不是防範惡意主機使用者的安全邊界。

## 重跑

在 Mac 上準備已下載的 LLVM 封存檔、既有的 `zstd`、`shasum` 和 Docker／OrbStack，執行：

```sh
bash docs/implementation/evidence/W-02-llvm-amd64-20261029/reproduce.sh /path/to/LLVM-23.1.4-Linux-X64.tar.zst
```

[重跑腳本](W-02-llvm-amd64-20261029/reproduce.sh)包含容器隔離選項、C 原始碼、編譯與執行步驟，SHA-256 為 `8572529db35c5a670b9dbe0026caf953fe11ee7967fbeb3387ce6cf23f6dd979`。2026-10-29 另以此腳本實跑一次並回傳 0，輸出與上述第三次輸出逐位元相同。容器限制 2 CPU、2 GiB 記憶體、128 個程序及 600 秒容器內執行時間；映像拉取不在此時間限制內。重跑需要下載固定映像及來源檔；腳本本身不下載來源檔，也不安裝 PHP 工具鏈。

## 待完成

尚未在原生 amd64 硬體驗證，尚未驗證 LLVM 與 Go runtime 的 ABI、記憶體管理、安全檢查或效能，也沒有因此宣稱 W-02 驗收通過。其他平台仍依 `compat.lock.json` 所列狀態追蹤。
