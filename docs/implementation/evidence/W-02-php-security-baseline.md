# W-02：PHP 8.5.12 參考來源的部分安全基準

對應 REQ-002、REQ-060、REQ-065 與 T-003、T-119、T-129 的準備工作。此紀錄是上游參考來源的公告核對，不是 Phego 引擎的漏洞修補或安全驗收。

2026-10-29 以 GitHub CLI 查閱 `php/php-src` 的兩份公開安全公告。兩份公告都將 8.5.12 列為修補版本；來源鎖的 PHP 版本也是 8.5.12。

| 公告 | 問題類型 | 上游列出的 PHP 8.5 修補版本 | Phego 驗證 |
|---|---|---|---|
| [GHSA-w8hw-w9fx-j79w / CVE-2026-109618](https://github.com/php/php-src/security/advisories/GHSA-w8hw-w9fx-j79w) | 記憶體不足時的狀態損毀 | 8.5.12 | `not_run` |
| [GHSA-6wmf-4rv5-p3rp / CVE-2026-109619](https://github.com/php/php-src/security/advisories/GHSA-6wmf-4rv5-p3rp) | 跨請求殘留狀態 | 8.5.12 | `not_run` |

這兩份公告分別於 2026-10-21 13:34:25 UTC 與 13:34:13 UTC 發布；[PHP 8.5.12 發布公告](https://www.php.net/releases/8_5_12.php)也標示該版本為安全更新。[PHP Foundation 說明](https://thephp.foundation/blog/2026/10/21/todays-php-security-release-and-how-were-strengthening-php-security/)提供兩項問題的背景。

可重查結構化資料：

```sh
gh api repos/php/php-src/security-advisories/GHSA-w8hw-w9fx-j79w
gh api repos/php/php-src/security-advisories/GHSA-6wmf-4rv5-p3rp
```

這只是兩項已查明的公告，不能視為全部已知安全事項。官方版本標示也不等於已針對本專案產物重現漏洞或核對所有修補內容。參考建置中各模組及執行環境的適用性、Go／LLVM 與容器映像的安全事項、Phego 的請求隔離與錯誤處理回歸都仍待確認。不可因參考來源被上游標示為已修補而宣稱 Phego 安全。
