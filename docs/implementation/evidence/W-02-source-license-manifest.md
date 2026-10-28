# W-02 來源與授權清單的部分整理

對應 REQ-060、T-119、T-120 的準備工作。[清單](W-02-source-license-manifest.json)由 [`source_license_manifest.py`](../../../tools/source_license_manifest.py) 整理來源鎖、PHP 和 Go 來源檔名清點、Go 與 LLVM 二進位封存檔候選清點，以及 PHP 參考建置映像與 Go／LLVM 探查映像的套件文件清點。狀態固定為 `partial`。

## 產生與檢查

在專案根目錄執行：

```sh
python3 tools/source_license_manifest.py generate --repo . > /tmp/phego-source-manifest.json
python3 tools/source_license_manifest.py check --repo . docs/implementation/evidence/W-02-source-license-manifest.json
```

2026-10-29 連續產生兩次，輸出逐位元相同，`check` 回傳 0。保存的 JSON SHA-256 為 `2a34c78603790a1594313d3f1563996f414a5add1de4003eacbf065e9539e090`。此結果只表示整理與比對成功，不代表授權或產品驗收通過。輸入檔案變更後須重新產生並檢查清單。

## 證據界線

清單記錄八份輸入的摘要，並核對已記錄的封存檔摘要、參考映像與套件數量。它不重新下載或核對封存檔，也不執行容器或辨識授權。清單中的檔名是候選資料，非 UTF-8 候選保留摘要和大小並列為未分類。

PHP 映像的 arm64 鎖定值是映像索引，清點中的 arm64 manifest 是該索引聲稱對應的子項目；此工具沒有重新讀取索引驗證這個對應。Python 工具映像的非 dpkg 內容、二進位封存檔與映像的完整授權審查、來源標頭與實際散布義務仍待確認。相關驗收保持未執行。
