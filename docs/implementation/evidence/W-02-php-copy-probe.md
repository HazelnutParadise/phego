# W-02 參考 PHP 來源快照探查

本紀錄對應 [REQ-002、REQ-060](../../data/phego-requirements.json)、局部情境 [T-003、T-119](../../data/phego-acceptance-tests.json) 與 [W-02](../W-02.md)。它不代表 Phego 相容或產品驗收。

[參考建置工具](../../../tools/reference_php.py)先將唯讀掛載的 PHP 封存檔複製到容器私人暫存區，再核對該份副本摘要並只解壓該副本。主機若在複製後改寫原始掛載檔，不會改變已核對的解壓輸入。`python3 -m unittest discover -s tools -p test_reference_php_archive_race.py` 的四項測試已通過；命令結構測試涵蓋 linux/arm64 與 linux/amd64。其餘三項動態測試使用 linux/arm64 profile 產生的腳本，在主機 shell 使用替身執行，未啟動 Docker。

2026-10-29 使用工具 SHA-256 `c0e890c59c30f67798c709b4e0671b7c98bbccd9c71d691feb5c5d06236f3a42` 與來源鎖 SHA-256 `44e58acca832e60f70eb9ee2ff2760256422c1aa4b7082b141b3c5fce651b144`，在 Mac 的 OrbStack Linux arm64 環境執行：

```sh
python3 tools/reference_php.py "$ARCHIVE" --platform linux/arm64 --timeout 1800
```

`ARCHIVE` 是已下載且符合來源鎖的 PHP 8.5.12 封存檔。命令回傳 0，CLI 執行檔 SHA-256 為 `e5d0591e0d82e30f413e4aa928ff66afddd1f752c707b17feffaae70c4f8b519`；上游 `tests/basic/001.phpt` 通過，失敗與跳過均為零。容器 ID `4ae45a271a933c03edc9ef40490c2eb6323aec6c9c5a39ad7783bbe28a096361` 經 `docker inspect` 檢查回傳 1 和 `no such object`。兩平台均只涵蓋 PHP 8.5.12，且各執行單項上游測試；本次修正後尚未比對相同鎖定輸入的第二次重建摘要，本紀錄也未附原始執行輸出。其他 PHP 版本、未涵蓋的作業系統／CPU 組合及完整產品行為仍待驗證。同日以 `python3 tools/reference_php.py "$ARCHIVE" --platform linux/amd64 --timeout 1800` 在 OrbStack 轉譯環境執行一次，回傳 0，CLI 執行檔 SHA-256 為 `04faa180577e391ed634d9b647ddd6c42d39cf82475e19b6f176d99d09811a9a`，上游 `tests/basic/001.phpt` 通過且失敗與跳過均為零。容器 ID `5fedd44c1dcced9b2140eaf1343360e03b1838e793ba45f07e6a91d4e7fe8fee` 經 `docker inspect` 檢查回傳 1 和 `no such object`。此結果不代表原生 amd64 或其他環境已驗證。

後續加強中斷及清理處理後，工具 SHA-256 為 `44e98785dd3400c3d30cbba098774e17845df7fc8ff2f92fccac8500e5a9cf38`。各平台產生的建置命令分別與上述工具版本相同。本段未記錄此工具版本的完整 PHP 重建結果。此版本以 arm64 執行 `--timeout 1` 回傳 124，容器 `63c53519bc874fcf9d378eed2729a34928ddc77e74bdadaffc93155bba3d0dd3` 經 inspect 確認不存在；另先以 inspect 確認容器 `fba220b8c7298ad1d09974f2c18263f5cff097c0fb0939c32743d1eb6f6502a3` 正在執行後送 SIGTERM，回傳 143，隨後 inspect 確認不存在。這些中斷探查不代表所有故障已實機驗證。
