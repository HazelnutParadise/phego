# W-02 官方發布來源追蹤

2026-10-28 使用 [PHP 官方發布資料](https://www.php.net/releases/index.php?json)與 [php-src 官方 GitHub](https://github.com/php/php-src) 取得維護中系列、各系列最新發布的原始碼摘要、公告、tag 物件與提交。已記錄在 [來源基準](../../data/php-upstream-baseline.json)：8.2.35、8.3.36、8.4.27、8.5.12。基準的 SHA-256 為 `371cac48a4dea46c78dd9e4d465e4774288b39f036e78457ba64bf0272da41f8`。

```sh
python3 tools/upstream_watch.py observe > observed.json
python3 tools/upstream_watch.py compare docs/data/php-upstream-baseline.json observed.json
```

兩次實際觀察與基準比較回傳 `unchanged`、`compatibility: not_assessed`，沒有候選變更。PHP 8.5.12 的原始碼封存檔已在[來源探查](W-02-source-probe.md)核對；其他系列此處只記錄官方發布資料所列的摘要，沒有下載核對封存檔。

工具追蹤已列出的維護中系列、發布 tag、原始碼摘要、公告網址與公告原始內容摘要；變化只列為待人工審查。它不解讀公告內容、不直接更新基準或來源鎖、不宣稱 PHP 語意相容，也不涵蓋所有歷史版本。公告網頁中的非內容變動也可能造成摘要改變。來源錯誤、限流或資料不完整會回報失敗，不會當成沒有變更。

GitHub API 的 `GITHUB_TOKEN` 為選用，只送往 `api.github.com`；未登入實測曾收到 HTTP 403 限流，使用既有 GitHub 憑證後重跑成功。不要把憑證寫入檔案或記錄。

[GitHub Actions 流程](../../../.github/workflows/upstream-watch.yml)預定每天執行，也會在來源鎖、基準或工具變更時執行。快照、差異與可用的錯誤診斷保留為 30 天工作流程產物；若有候選變更則工作失敗並保留產物供審查。本機以無效測試憑證模擬查詢失敗，確認回傳失敗、診斷留存且未包含憑證內容；GitHub 產物上傳尚待遠端執行驗證。排程要等流程進入預設分支後才會運作，GitHub 排程可能延遲；目前尚未證明排程實際執行。

關聯需求 REQ-065，局部對照 T-129／T-130；完整語意差異分類、人工審查流程與多版本回歸仍待 W-45。
