# W-49｜早期 PHP 版本的來源線索

此紀錄對應 REQ-080 與 T-159、T-160，只是提前進行的唯讀來源發現，不是完整公開版本清冊或歷史相容性驗收。兩項情境仍為 `not_run`。

2026-10-29 查核下列原始資料：

- [1995-06-08 Rasmus Lerdorf 的 Usenet 公告](https://groups.google.com/g/comp.infosystems.www.authoring.cgi/c/PyJ25gZ6z7A/m/M9FkTUVDfcwJ)宣布 Personal Home Page Tools 1.0，介紹 C 語言 CGI 工具，並指向作者當時的首頁；公告沒有列出來源封存檔名稱或摘要。
- [Rasmus 的 2002 年歷史投影片](https://talks.php.net/show/oscon2002/5)稱 1995-06-08 是第一個正式 1.0 發布，並列出 1995-09-18 的 1.90、10-09 的 1.91、10-17 的 1.92 及後續版本。這是尋找版本的線索，不足以單獨證明每個所列版本都曾公開發布或確認其來源內容。
- [PHP 官方歷史手冊](https://www.php.net/manual/en/history.php.php)描述 1995 年的 PHP Tools、FI 與 Construction Kit，並將 PHP/FI 名稱的介紹放在 1996 年 4 月。投影片則把 1995-10-17 的 1.92 稱為首個公開 PHP/FI 版本。兩種命名及時間描述尚未逐項調和，不據此合併或排除版本。
- 查核時 [官方 museum 的 PHP 1 目錄](https://museum.php.net/php1/)僅列出 `php-108.tar.gz`。該目錄顯示的 2014 年日期不能視為 1995 年發布日期。既有[封存檔探查](W-49-php-108-source.json)確認其中 `version.h` 標示 1.0.8；它不能代表 1.0 首版。


## 已排除的相似標籤

[php-src 的 `RELEASE_1_0_1` 標籤](https://github.com/php/php-src/tree/RELEASE_1_0_1)不能當作 1995 年 PHP Tools 1.0 或 1.0.1 的來源證據。2026-10-29 查核 [Git 標籤物件](https://api.github.com/repos/php/php-src/git/tags/b939aea540d8885002e08796895cebbc3e423b15)及其[提交樹](https://github.com/php/php-src/tree/6c332260393eef23ec1e7c12de96b6514b7e2a7a)：該標籤由 CVS 匯入建立，標籤時間為 2007-03-28，提交樹包含 `ext` 與 `pear`。其中 [Fileinfo 套件紀錄](https://github.com/php/php-src/blob/6c332260393eef23ec1e7c12de96b6514b7e2a7a/ext/fileinfo/package.xml)列出 2005 年的 1.0.1 與 PHP 5.1 相容資訊。另一個名稱相似的 [`RELEASE_1_0` 標籤物件](https://api.github.com/repos/php/php-src/git/tags/349792e181384f1b500399eed5250e07007769c5)時間為 2005-12-06，指向的[提交樹](https://github.com/php/php-src/tree/c7fdb98bc59fe743c682e1362456474da6c92402)同樣只有 `ext` 與 `pear`，也不能作為 1995 首版來源。這些查核不能推論首版來源已不存在。

公告同時使用 public domain 與 GNU Public License 字樣，不能單憑這段文字決定首版的授權或再散布條件。1.0 來源封存檔、完整公開版本清冊、各版授權與可執行參考環境均尚未確認。需繼續尋找可核對身分的原始來源，保留缺失而不縮小歷史相容範圍。
