# W-02：參考 PHP 建置 profile 的部分驗證

對應 REQ-002、REQ-060、T-003、T-119 的局部證據。參考建置會比對執行中的 PHP 版本、整數大小、CLI、作業系統、架構，以及 `-n` 未載入或掃描 INI；擴充與 session 欄位只作觀察，沒有宣稱完整模組或安全政策驗收。

2026-10-29 使用的 `tools/reference_php.py` SHA-256 為 `86d65523b40cec08d5e7749b15bc136a4c7cd8acc5cecb93501b30f6ed8f02c2`，`compat.lock.json` 為 `bf3f83687aa52c04847aaa15ce07ed5190d8780782b6adb307d567bfba33385f`，PHP 來源封存檔為 `08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209`。容器在 OrbStack 中無網路、非 root 執行，參考工具保留記憶體、CPU、程序數、時限與清理控制；退出碼與容器清理結果來自當次操作紀錄，不能只憑原始輸出重證。PHP 與工具鏈未安裝在 Mac 上。

## Linux arm64

`python3 tools/reference_php.py ARCHIVE --platform linux/arm64 --timeout 900` 回傳 0，profile 符合 PHP 8.5.12、64 位元、Linux aarch64、CLI 與無 INI 載入；單項 `tests/basic/001.phpt` 通過。執行檔 SHA-256 為 `e5d0591e0d82e30f413e4aa928ff66afddd1f752c707b17feffaae70c4f8b519`。[原始輸出](W-02-php-profile-20261029/linux-arm64.log.gz)解壓後 SHA-256 為 `8e66f214ae9a420638ebd3f24cd1a5e9bee9a14ec15712533b2a9ddaf5e2758b`，其中列出實際一般及 Zend 擴充；此設定未載入 session 擴充。

這是參考 PHP 局部探查，不能代表 Phego 引擎、完整歷史版本、其他 SAPI、效能或安全回歸已完成。上游安全公告與尚未驗證的事項另見[部分安全基準](W-02-php-security-baseline.md)。
