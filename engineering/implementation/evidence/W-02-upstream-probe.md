# W-02 上游來源差異探查

本次對照 REQ-065、T-129、T-130，執行的是局部來源差異探查，不是完整情境驗收。參考 PHP 8.5.11 與 8.5.12 官方來源及其發布標籤；版本、tag 物件、提交、封存檔 SHA-256 和公告來源如下。

| PHP | tag 物件 | 提交 | 封存檔 SHA-256 |
|---|---|---|---|
| 8.5.11 | `597eb0b4e32cc8802e112fe13cc92fb5636a1781` | `678778973bb4d4185c06ef1320de50f1a975d3c8` | `d9be75c08e8c316f4c8f4194d8fbe1750a15f6a6d9d4e3fe72082abeeb800360` |
| 8.5.12 | `c96586077b050b362744ba295f97f6395f58a75a` | `136e049ac10b032c83018e65b63b6df858943750` | `08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209` |

摘要來自 [PHP 8.5 官方發布資料](https://www.php.net/releases/index.php?json&version=8.5&max=5)，並與實際下載封存檔比對；tag 物件與提交由 `git ls-remote https://github.com/php/php-src.git` 查得。公告線索分別是 [8.5.11](https://www.php.net/releases/8_5_11.php) 與 [8.5.12](https://www.php.net/releases/8_5_12.php)。

對已下載檔案設定 `OLD_ARCHIVE` 與 `NEW_ARCHIVE`，以新建的空目錄執行：

```sh
python3 tools/upstream_snapshot.py capture --archive "$OLD_ARCHIVE" --sha256 d9be75c08e8c316f4c8f4194d8fbe1750a15f6a6d9d4e3fe72082abeeb800360 --version 8.5.11 --tag php-8.5.11 --commit 678778973bb4d4185c06ef1320de50f1a975d3c8 --announcement https://www.php.net/releases/8_5_11.php > old.json
python3 tools/upstream_snapshot.py capture --archive "$NEW_ARCHIVE" --sha256 08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209 --version 8.5.12 --tag php-8.5.12 --commit 136e049ac10b032c83018e65b63b6df858943750 --announcement https://www.php.net/releases/8_5_12.php > new.json
python3 tools/upstream_snapshot.py compare old.json new.json > diff.json
```

快照分別為 22,007 和 22,415 個來源項目，實際檔案 SHA-256 分別為 `b7ace91fae6ffe0d3a8ada394ad138900cc002fe2c99488d23b055577fa72a15` 與 `5c798f1bd1f18ccd2005fba37f4c409a7892b557e52b2f1bbc8e9a3727fc4b6e`。[完整差異記錄](W-02-upstream-8.5.11-8.5.12.json)的 SHA-256 是 `55f1041efb9a4fa43ee379f78e6451e50c3ba1e45a696cfe682857cf114cbe37`；再次擷取 8.5.11 及比較兩份快照得到相同摘要。

來源差異為測試新增 422、移除 14、內容變更 37，stub 內容變更 3，發布紀錄內容變更 2；兩個公告網址不同。語法檔沒有內容差異。`compatibility` 明確為 `not_assessed`。

工具不執行封存內容；封存檔、展開大小、項目數及延伸標頭都有上限，並拒絕連結、特殊檔與 sparse 項目。工具不自行下載來源或驗證輸入 tag、提交和公告的官方身分；這些身分依上述外部資料另行核對。公告欄位只比較網址，沒有分析公告內容或自動判定安全影響。此工具目前針對具有上述目錄結構的 PHP 來源，未覆蓋所有歷史版本、排程、語意差異分類或多版本回歸。
