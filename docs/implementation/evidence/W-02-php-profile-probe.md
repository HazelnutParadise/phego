# W-02：參考 PHP 建置 profile 的部分驗證

對應 REQ-002、REQ-060、T-003、T-119 的局部證據。參考建置會比對執行中的 PHP 版本、整數大小、CLI、作業系統、架構，以及 `-n` 未載入或掃描 INI；舊版只觀察擴充與 session 欄位，新版會依來源鎖核對；這些檢查不代表完整模組或安全政策驗收。

2026-10-29 使用的 `tools/reference_php.py` SHA-256 為 `86d65523b40cec08d5e7749b15bc136a4c7cd8acc5cecb93501b30f6ed8f02c2`，`compat.lock.json` 為 `bf3f83687aa52c04847aaa15ce07ed5190d8780782b6adb307d567bfba33385f`，PHP 來源封存檔為 `08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209`。容器在 OrbStack 中無網路、非 root 執行，參考工具保留記憶體、CPU、程序數、時限與清理控制；退出碼與容器清理結果來自當次操作紀錄，不能只憑原始輸出重證。PHP 與工具鏈未安裝在 Mac 上。

## Linux arm64

`python3 tools/reference_php.py ARCHIVE --platform linux/arm64 --timeout 900` 回傳 0，profile 符合 PHP 8.5.12、64 位元、Linux aarch64、CLI 與無 INI 載入；單項 `tests/basic/001.phpt` 通過。執行檔 SHA-256 為 `e5d0591e0d82e30f413e4aa928ff66afddd1f752c707b17feffaae70c4f8b519`。[原始輸出](W-02-php-profile-20261029/linux-arm64.log.gz)解壓後 SHA-256 為 `8e66f214ae9a420638ebd3f24cd1a5e9bee9a14ec15712533b2a9ddaf5e2758b`，其中列出實際一般及 Zend 擴充；此設定未載入 session 擴充。

## Linux amd64 轉譯

`python3 tools/reference_php.py ARCHIVE --platform linux/amd64 --timeout 900` 在 OrbStack 轉譯環境回傳 0，profile 符合 PHP 8.5.12、64 位元、Linux x86_64、CLI 與無 INI 載入；單項 `tests/basic/001.phpt` 通過。執行檔 SHA-256 為 `04faa180577e391ed634d9b647ddd6c42d39cf82475e19b6f176d99d09811a9a`。[原始輸出](W-02-php-profile-20261029/linux-amd64.log.gz)解壓後 SHA-256 為 `08b8671b48dccb09922208e6674b3a6d480f6c711bb5c5281cff457427587868`。此設定也未載入 session 擴充。這不是原生 amd64 硬體驗證。

兩次容器均回傳成功，其容器 ID 在完成後經 `docker inspect` 確認不存在。退出碼與後續 inspect 結果屬當次操作紀錄。

## 鎖定擴充與 session 欄位

依上述兩平台的實際觀察，各自的 `expected_profile` 已加入來源鎖。新版參考建置會比對一般及 Zend 擴充的完整集合，拒絕新增、缺少或重複項目，並嚴格比對 session 欄位的型別與值。這些值只適用固定的 PHP 8.5.12、映像、設定與 CLI `-n`，不是 Phego 的能力承諾。

新版工具 SHA-256 為 `08265060e296ec9ef6081d9042060d959f3cb1b99f01676a8cc22d8c62750754`，新版來源鎖 SHA-256 為 `a22d697ec519d6a5a612c344be26cf5ef9c5389b279e440659094d11fc0b1d4a`。2026-10-29 Linux arm64 以同一命令重新建置，擴充及 session 欄位比對通過，單項 PHPT 通過；[原始輸出](W-02-php-profile-20261029/linux-arm64-locked.log.gz)解壓後 SHA-256 為 `ee40ac3f0f6d17bcbc98a1265b37fb1b025fd411644e9040dd9450b7d0a24f8c`。OrbStack 轉譯 amd64 也以新版工具重新建置，擴充與 session 欄位比對及單項 PHPT 均通過；[原始輸出](W-02-php-profile-20261029/linux-amd64-locked.log.gz)解壓後 SHA-256 為 `297e66d7e585d5911b131fa8e25433b6cd6e7a95613adcc0b195d98d94d2339a`。兩次新版容器均已確認移除；amd64 仍非原生硬體驗證。

兩個 session 欄位為 `false` 是因這個參考建置沒有載入 session，`ini_get` 也無該設定；不能據此宣稱 upload progress 已停用或 session 安全回歸已通過。

這是參考 PHP 局部探查，不能代表 Phego 引擎、完整歷史版本、其他 SAPI、效能或安全回歸已完成。上游安全公告與尚未驗證的事項另見[部分安全基準](W-02-php-security-baseline.md)。
