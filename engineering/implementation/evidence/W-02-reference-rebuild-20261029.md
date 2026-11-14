# W-02｜現行參考 PHP 工具重建探查

此紀錄對應 REQ-002、REQ-060、T-003、T-119 與 [W-02](../W-02.md)。只驗證參考建置，不代表 Phego 相容或產品驗收；情境仍為 `not_run`。

2026-10-29 在 Mac 的 OrbStack Linux arm64 虛擬機，以提交 `c108e7d56cff7d783274e465ee4326808ffd80f1` 的 `tools/reference_php.py` 執行。工具 SHA-256 `44e98785dd3400c3d30cbba098774e17845df7fc8ff2f92fccac8500e5a9cf38`，`compat.lock.json` SHA-256 `44e58acca832e60f70eb9ee2ff2760256422c1aa4b7082b141b3c5fce651b144`；PHP 8.5.12 封存檔 SHA-256 `08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209`。Docker Server 29.8.2，核心 `7.2.12-orbstack-00394-gbecdd6130068`。工具使用固定映像、唯讀來源掛載、無網路與受限容器，複製並核對來源後才解壓。`ARCHIVE` 需指向來源鎖所列且已下載的封存檔：

```sh
python3 tools/reference_php.py "$ARCHIVE" --platform linux/arm64 --timeout 1800
```

在相同平台與鎖定輸入下，以上命令於兩個新容器各執行一次，兩次回傳 0，CLI 二進位 SHA-256 均為 `e5d0591e0d82e30f413e4aa928ff66afddd1f752c707b17feffaae70c4f8b519`，且 `tests/basic/001.phpt` 各通過 1 項，失敗與跳過均為 0。兩次容器 ID 依序為 `bd3bb6d7c13eb9075b54d0f510293b3689ed314f9fdf7de40ffa438e142de131`、`af513336f0e6e7f27499a374a4b5d7262a0f3ab905fd4f445687f63b246bf24f`；各自執行 `docker inspect <ID>` 均回傳 1 和 `no such object`。

兩次完整標準輸出與標準錯誤合併保存為壓縮原始紀錄：[第一次](W-02-reference-rebuild-20261029/linux-arm64-1.log.gz)、[第二次](W-02-reference-rebuild-20261029/linux-arm64-2.log.gz)。解壓後的 SHA-256 依序為 `24a5ea2ac26fbf4621ca27dff847810c866d64074ea19cbf433fd4877ce8ccc5`、`bedb020dffa637dc7ea197165583323ea031f38a9df14be9f16a7c889354401d`；執行命令的結束碼和後續 inspect 結果記錄於上段，不包含在壓縮輸出中。

## Linux amd64 轉譯環境

以相同工具、來源與來源鎖，將上述命令的 `--platform` 改為 `linux/amd64`，在 OrbStack 轉譯環境中使用兩個新容器各執行一次。兩次均回傳 0，CLI 執行檔 SHA-256 均為 `04faa180577e391ed634d9b647ddd6c42d39cf82475e19b6f176d99d09811a9a`，且 `tests/basic/001.phpt` 各通過 1 項，失敗與跳過均為 0。兩次容器 ID 依序為 `7f3a45922e43197dcddf991ec3220a92b75ccbe702c15f36f4fd2269aa336469`、`0e729d3df52b19666f4a082a17f63d3f61c6d7f7d35328ecb2fe5957c4a3a957`；各自 `docker inspect` 均回傳 1 和 `no such object`。

[第一次](W-02-reference-rebuild-20261029/linux-amd64-1.log.gz)及[第二次](W-02-reference-rebuild-20261029/linux-amd64-2.log.gz)壓縮檔保留標準輸出與標準錯誤合併的原始結果，解壓後 SHA-256 依序為 `bbcd85af17b8642f2492ab0fa8aa8d615f17adf108c07a75dbb81f2dab80daa7`、`e449b12cf82d56ba42feb89a9708620931cda41254a9d470b922613f7139544d`。結束碼及 inspect 結果未包含在壓縮輸出中。

上述比較只涵蓋同一 OrbStack 核心、Linux arm64 與轉譯 amd64、單一 PHP 版本、建置設定與一項上游測試。它不證明原生 amd64、其他作業系統、CPU、核心或 PHP 版本的重現性，也不證明完整上游測試或應用相容性。
