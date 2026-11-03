# W-02｜本票參考基準的來源、授權與安全紀錄

這份清單只涵蓋 W-02 取得、建置或執行的固定參考來源與工具，不代表 Phego 產品已支援 PHP，也不構成軟體發布或第三方材料散布許可。版本、網址和摘要以 [compat.lock.json](../../../compat.lock.json) 為準。

| 用途與來源 | 已核對的授權文件與保留方式 | 界線 |
|---|---|---|
| PHP 8.5.12 官方來源及內含 PHPT 測試 | 封存檔主 `LICENSE` 標示 PHP License 3.01；另有 Zend Engine License、Lexbor 的 Apache-2.0 `LICENSE` 和 `NOTICE` 等。保留原始封存檔的文件、版權標記及通知；17 份候選文件與原文觀察見 [PHP 授權文件](W-02-php-license-observations.md)。 | 授權不只一種；未逐檔完成實際產物的適用範圍與散布審查。 |
| Go 1.27.3 來源與各平台 bootstrap | 官方封存檔中 `go/LICENSE` 為 BSD 授權文件，另有 `go/PATENTS` 與內嵌第三方授權；保留來源與各自的授權及專利文件。文件路徑及摘要見 [Go 來源清點](W-02-go-source-license-files.json)與[二進位清點](W-02-binary-license-files.json)。 | 測試只使用固定工具鏈；未認定所有內嵌元件適用於哪個最終產物。 |
| LLVM／LLD 23.1.4 原生概念驗證 | 固定提交的 [LLVM LICENSE.TXT](https://github.com/llvm/llvm-project/blob/7a941dfb2877a0b8cf30323fefa85ad019e0b0f5/llvm/LICENSE.TXT) 標示 Apache License 2.0 with LLVM exceptions；保留原始授權及第三方通知。封存檔候選文件見 [二進位清點](W-02-binary-license-files.json)。 | 僅用於可選原生工具探查；未完成所有附帶元件的適用性與散布審查。 |
| 參考建置和清點容器映像 | 固定映像及平台、套件名稱、版本與各 `copyright` 文件摘要見[來源清單](W-02-source-license-manifest.json)。保留該清單與各映像中的原始文件。 | 套件文件摘要不是授權判定；映像非 dpkg 內容與實際產品散布義務仍未完整審查。 |
| Phego 原創程式及文件 | 保留專案根目錄 [MIT LICENSE](../../../LICENSE)。 | PHP、Go、LLVM 和映像的第三方授權不因此變成 MIT。 |

[整合清單](W-02-source-license-manifest.md)維持 `partial`，以保留未判定的第三方範圍。這份文件補齊本票「固定參考來源及其授權文件如何追溯」的交接；W-44 在決定實際發布內容前仍須核對所含元件、授權條款及通知，不能把這份交接當成發布核准。

## 已知安全資訊與查詢限制

- PHP 8.5.12 的兩項上游安全公告與本地尚未執行的 Phego 驗證見 [PHP 安全基準](W-02-php-security-baseline.md)。
- Go 官方[發布紀錄](https://go.dev/doc/devel/release)記載 1.27.3 含 `crypto/x509` 安全修正；[事件說明](https://go.dev/blog/emudhra-incident)說明此版本對受影響中繼憑證的處理。這不表示所有 Go 漏洞已排除，也不證明 Phego 安全。
- 2026-10-29 查詢 [LLVM 安全公告庫](https://github.com/llvm/llvm-security-repo/security/advisories)，取得四筆公開紀錄，其中一筆自述為測試；其餘三筆列出的受影響範圍均不含鎖定的 23.1.4。這只比較公告所列版本範圍，未重現漏洞，亦非完整 LLVM 漏洞清單。[API 原始輸出](W-02-llvm-advisories-20261029.json.gz)解壓後 SHA-256 為 `da8d617517e2451b537db6a70a5081963f195bd45e89e2aa2866a816a5cf0ac1`。
- 固定映像套件的完整安全公告、其他資料來源與之後的新公告仍未完整分析。不能把缺少公告當成無漏洞；持續發現與完整行為比較由 W-45 處理。
