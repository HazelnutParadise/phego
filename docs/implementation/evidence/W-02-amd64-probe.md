# W-02 Linux amd64 參考建置探查

對應 REQ-002、REQ-060 與局部情境 T-003、T-119；這不是 Phego 引擎或完整情境驗收。Mac 的 OrbStack Docker Server 回報 Linux arm64，以下 amd64 容器透過轉譯執行，不能代表原生 amd64 主機的結果。

已固定的 amd64 映像為 `buildpack-deps@sha256:702b53c861e19913b8720993a4032b13441ed63916e123b403301423977f818f`。上游多架構索引 `sha256:2d7fd9aab90c5444e6c4f5c59fce9ab6ba6bce7b8709ef74fcd1f618cd2e867b` 中的 Linux amd64 項目指向此摘要；容器 `uname -m` 回報 `x86_64`。容器內 Debian 12.15、GCC 12.2.0、GNU ld 2.40、GNU Make 4.3。

`tools/reference_php.py` 的預設平台仍是 `linux/arm64`。指定 amd64 及較長期限的命令如下，`$ARCHIVE` 是已下載的 PHP 8.5.12 封存檔：

```sh
python3 tools/reference_php.py "$ARCHIVE" --platform linux/amd64 --timeout 1800
```

兩個平台各執行一次 `--timeout 1`，均回傳 124，並以 Docker inspect 確認各自容器已移除。此測試只驗證期限與清理，不代表完整建置成功。完整 amd64 參考引擎建置與上游 PHPT 結果尚待記錄，原生 amd64 主機也尚未驗證。
