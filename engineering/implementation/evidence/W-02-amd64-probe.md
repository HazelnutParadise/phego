# W-02 Linux amd64 參考建置探查

對應 REQ-002、REQ-060 與局部情境 T-003、T-119；這不是 Phego 引擎或完整情境驗收。Mac 的 OrbStack Docker Server 回報 Linux arm64，以下 amd64 容器透過轉譯執行，不能代表原生 amd64 主機的結果。

已固定的 amd64 映像為 `buildpack-deps@sha256:702b53c861e19913b8720993a4032b13441ed63916e123b403301423977f818f`。上游多架構索引 `sha256:2d7fd9aab90c5444e6c4f5c59fce9ab6ba6bce7b8709ef74fcd1f618cd2e867b` 中的 Linux amd64 項目指向此摘要；容器 `uname -m` 回報 `x86_64`。容器內 Debian 12.15、GCC 12.2.0、GNU ld 2.40、GNU Make 4.3。

`tools/reference_php.py` 的預設平台仍是 `linux/arm64`。指定 amd64 及較長期限的命令如下，`$ARCHIVE` 是已下載的 PHP 8.5.12 封存檔：

```sh
python3 tools/reference_php.py "$ARCHIVE" --platform linux/amd64 --timeout 1800
```

兩個平台以 `python3 tools/reference_php.py "$ARCHIVE" --platform linux/arm64 --timeout 1` 和 `python3 tools/reference_php.py "$ARCHIVE" --platform linux/amd64 --timeout 1` 各重跑一次，均回傳 124。腳本清理輸出分別為容器 ID `2a3f577bab24e5f4dc8a694ef1184e2dcbdf8eb395f1cf1b712f7f3b3e1f5344` 和 `700533625e4ea712df6a66b90177a9abca59fbfe03d6419286bff990151319b6`；對兩者執行 `docker inspect <容器ID>` 均回傳 1 與 `no such object`。重跑時應以當次輸出的 ID 核對，Docker 無法回應時不得將錯誤當成清理成功。此測試只驗證期限與清理，不代表完整建置成功。原生 amd64 主機尚未驗證。

## 手動完整建置

以同一上游多架構索引、`--platform linux/amd64` 與固定主機名稱執行一次乾淨參考建置。來源摘要核對成功，PHP 8.5.12 CLI 產物 SHA-256 為 `04faa180577e391ed634d9b647ddd6c42d39cf82475e19b6f176d99d09811a9a`，上游 `tests/basic/001.phpt` 一項通過，失敗與跳過均為零。命令回傳 0，對當次容器 ID `f3c8dba05b35` 執行 `docker inspect f3c8dba05b35`，回傳 1 與 `no such object`。重跑時可在下面的 `docker run` 加入指向不存在檔案的 `--cidfile`，完成後以檔案內的當次 ID 執行 `docker inspect`。這是轉譯環境中的單次手動建置，不代表跨環境可重現或 Phego 相容。

```sh
docker run --rm --pull never --platform linux/amd64 --hostname phego-reference \
  --network none --read-only --tmpfs /tmp:rw,nosuid,nodev,exec,size=2g \
  --user 65534:65534 --pids-limit 256 --memory 2g --cpus 2 --cap-drop ALL \
  --security-opt no-new-privileges \
  --mount "type=bind,source=$ARCHIVE,target=/input/php.tar.xz,readonly" \
  buildpack-deps@sha256:2d7fd9aab90c5444e6c4f5c59fce9ab6ba6bce7b8709ef74fcd1f618cd2e867b \
  timeout --kill-after=5s 1800s sh -ec 'export SOURCE_DATE_EPOCH=1792540800 TZ=UTC LC_ALL=C; printf "08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209  /input/php.tar.xz\n" | sha256sum -c -; mkdir -p /tmp/build; cd /tmp/build; tar -xf /input/php.tar.xz; cd php-8.5.12; ./configure --disable-all --enable-cli --without-pear > /tmp/configure.log 2>&1 || { tail -60 /tmp/configure.log; exit 1; }; make -j2 > /tmp/make.log 2>&1 || { tail -60 /tmp/make.log; exit 1; }; ./sapi/cli/php -n -v; sha256sum ./sapi/cli/php; TEST_PHP_EXECUTABLE=./sapi/cli/php ./sapi/cli/php -n run-tests.php -P tests/basic/001.phpt'
```

## 腳本完整建置

使用上述腳本命令完整建置一次，腳本 SHA-256 為 `98c710cc26f6212dfda73b68c3c03c840e723309cb9194fe3ec397b5935a6515`。PHP CLI 摘要為 `04faa180577e391ed634d9b647ddd6c42d39cf82475e19b6f176d99d09811a9a`，與本次相同環境下的手動建置一致；`tests/basic/001.phpt` 通過，失敗與跳過均為零。命令回傳 0，當次清理輸出容器 ID `a7f5975f4980a704e01253fbe30a49681baa402f9b7f0c590357c633c0aecfbd`，以 `docker inspect` 檢查該 ID 回傳 1 與 `no such object`。兩次結果僅支持此 OrbStack 轉譯環境、來源與建置設定下的比較，不能推論原生主機或其他環境的重現性，也不代表 Phego 相容。
