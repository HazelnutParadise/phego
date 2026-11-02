# W-02 來源與授權清單的部分整理

對應 REQ-060、T-119、T-120 的準備工作。[清單](W-02-source-license-manifest.json)由 [`source_license_manifest.py`](../../../tools/source_license_manifest.py) 整理來源鎖、PHP 和 Go 來源檔名清點、Go 與 LLVM 二進位封存檔候選清點，以及 PHP 參考建置映像與 Go／LLVM 探查映像的套件文件清點。狀態固定為 `partial`。

## 產生與檢查

在專案根目錄執行：

```sh
python3 tools/source_license_manifest.py generate --repo . > /tmp/phego-source-manifest.json
python3 tools/source_license_manifest.py check --repo . docs/implementation/evidence/W-02-source-license-manifest.json
```

2026-10-29 連續產生兩次，輸出逐位元相同，`check` 回傳 0。本次 macOS arm64 探查後重新產生，保存的 JSON SHA-256 為 `03d4cfac0c8de655435350aa66bcc90d74281ebb758c38096815416ea9ae7a4a`。此結果只表示整理與比對成功，不代表授權或產品驗收通過。輸入檔案變更後須重新產生並檢查清單。

## 證據界線

清單記錄十份輸入的摘要，並核對已記錄的封存檔摘要、參考映像與套件數量。它不重新下載或核對封存檔，也不執行容器或辨識授權。清單中的檔名是候選資料，非 UTF-8 候選保留摘要和大小並列為未分類。

PHP 參考映像的[原始索引](W-02-php-reference-index.json)已依來源鎖核對 SHA-256，並驗證 arm64 與 amd64 平台各自的 manifest 對應。Python 工具映像的非 dpkg 內容雖已有部分檔名清點，其餘內容、二進位封存檔與映像的完整授權審查、來源標頭與實際散布義務仍待確認。相關驗收保持未執行。

固定參考用途的元件、授權文件、通知保留方式與安全資訊見[參考基準](W-02-reference-baseline.md)。清單中的 `partial` 保留尚未完成的逐檔適用性與發布審查。

Windows Go 的鎖定狀態為 `native_vm_pure_go_build_passed`，清單會辨識此值；[實際結果](W-02-windows-go-source-20261029.md)顯示虛擬機測試已通過，但未封鎖網路。
