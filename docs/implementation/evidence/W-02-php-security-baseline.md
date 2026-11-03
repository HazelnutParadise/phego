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

## 尚待確認的上游問題回報

2026-10-29 查閱 `php/php-src` 的 issue 與留言時，下列問題仍為開啟狀態。它們是回報者提供的結果，尚未由本專案重現，也不能據此認定本專案固定的 CLI 建置必然受影響。

| 上游 issue | 回報內容與條件 | 本專案判定 |
|---|---|---|
| [#24481](https://github.com/php/php-src/issues/24481) | `unserialize()` 中解構函式重新進入時可能讀取已釋放記憶體；[留言](https://github.com/php/php-src/issues/24481#issuecomment-6338542073)聲稱在 8.5.12 等版本的標準建置上重現。[#24672](https://github.com/php/php-src/issues/24672) 已作為重複回報關閉。 | `unverified`；未執行上游測試程式，尚未判定固定 CLI 建置的適用性。 |
| [#24787](https://github.com/php/php-src/issues/24787) | 回報者以特定記憶體與 multipart 設定測試 8.5.12 的 FPM 與 Apache，觀察到工作程序退出及暫存檔殘留。 | `unverified`；目前固定參考建置只驗證 CLI，不能推論 FPM 或 Apache 的結果適用於它。 |
| [#24802](https://github.com/php/php-src/issues/24802) | 回報者指出 multipart 剖析期間錯誤處理函式重入 `request_parse_body()` 可能造成記憶體重複釋放，並回報 8.5.12 CLI 測試結果。 | `unverified`；未重現或審核觸發條件與固定 CLI 設定的適用性。 |

這些 issue 不等同已發布的安全公告或已確認的 Phego 漏洞。後續應在無祕密、無正式資料、受資源限制的拋棄式環境，核對來源與重現條件後再決定差異測試或漏洞回歸；不能把未查證的上游行為當成 Phego 相容基準。
