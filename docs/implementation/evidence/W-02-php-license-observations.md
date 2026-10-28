# W-02｜PHP 來源授權文件的原文觀察

此紀錄對應 REQ-060、T-119 與 [W-02](../W-02.md)。2026-10-29 在無網路、唯讀、非 root 且限制資源的 Linux 容器中，核對 PHP 8.5.12 封存檔 SHA-256 `08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209`，並逐一核對[既有清單](W-02-php-license-files.json)的 17 份一般檔案大小與摘要，均相符。容器映像為 `buildpack-deps@sha256:4c9dc8ac4ff58f6fe23d2b007fe3bb8e25716f947adae6fb312d6cfbaac2b3c3`；沒有執行來源程式。清單保留每份文件的完整路徑與摘要，路徑以下省略 `php-8.5.12/`。

| 文件 | 原文中可確認的內容 |
|---|---|
| `LICENSE` | 標示 PHP License 3.01，並提到 Zend Engine。 |
| `TSRM/LICENSE` | 有來源散布與名稱使用條款，未自稱授權名稱。 |
| `Zend/LICENSE` | 標示 Zend Engine License 2.00。 |
| `Zend/asm/LICENSE` | 標示 Boost Software License 1.0。 |
| `ext/bcmath/libbcmath/LICENSE` | 是 LGPL 2.1 通用文字，套用範例保留未填模板；不能僅依範例認定整個元件的版本選擇。 |
| `ext/date/lib/LICENSE.rst` | 標示 MIT。 |
| `ext/fileinfo/libmagic/LICENSE` | 有來源及二進位散布條款，未自稱授權名稱。 |
| `ext/gd/tests/Rochester-Regular.otf.LICENSE.txt` | 是 Apache License 2.0 通用文字，權利人範例仍為模板。 |
| `ext/lexbor/LICENSE` | 是 Apache License 2.0 通用文字。 |
| `ext/lexbor/NOTICE` | 標示 Lexbor、Alexander Borisov 與 Apache License 2.0；這是一份獨立通知。 |
| `ext/mbstring/libmbfl/LICENSE` | 含 LGPL 2.1 與 HappySize 關於 streamable kanji code filter and converter 的聲明，另有日文條件。 |
| `ext/opcache/jit/ir/LICENSE` | 標示 MIT。 |
| `ext/standard/libavifinfo/LICENSE` | 列出 Alliance for Open Media 及散布條款，未自稱授權名稱。 |
| `ext/uri/uriparser/COPYING.BSD-3-Clause` | 檔名含 BSD-3-Clause，內文識別 uriparser 並列散布條件。 |
| `ext/uri/uriparser/include/uriparser/COPYING` | 與前一份內容及摘要相同。 |
| `ext/uri/uriparser/src/COPYING` | 與前兩份內容及摘要相同。 |
| `sapi/fpm/LICENSE` | 列出 Andrei Nigmatulin 及散布條款，未自稱授權名稱。 |

這些觀察不等於各原始檔、外部依賴或實際產物的完整授權歸屬，也不判定分發義務；原始檔內嵌標頭、適用範圍及完整條款仍待審查。不同文件出現不同授權，不代表整包可任選授權。T-119 仍為 `not_run`。

重跑下列核對時，`ARCHIVE` 為已下載的 PHP 8.5.12 封存檔絕對路徑，`INVENTORY` 為上述清單的絕對路徑。輸出會包含授權原文，供逐列核對。

```sh
docker run --rm -i --pull never --platform linux/arm64 --network none --read-only \
  --user 65534:65534 --pids-limit 32 --memory 256m --cpus 1 \
  --cap-drop ALL --security-opt no-new-privileges \
  --mount "type=bind,src=$ARCHIVE,dst=/input/php.tar.xz,readonly" \
  --mount "type=bind,src=$INVENTORY,dst=/input/inventory.json,readonly" \
  buildpack-deps@sha256:4c9dc8ac4ff58f6fe23d2b007fe3bb8e25716f947adae6fb312d6cfbaac2b3c3 \
  python3 - <<'PYCODE'
import hashlib, json, tarfile
from pathlib import Path
inventory = json.loads(Path('/input/inventory.json').read_text())
archive = Path('/input/php.tar.xz')
if hashlib.sha256(archive.read_bytes()).hexdigest() != inventory['archive_sha256']:
    raise SystemExit('archive digest mismatch')
with tarfile.open(archive, 'r:xz') as source:
    for item in inventory['files']:
        member = source.getmember(item['path'])
        if not member.isfile():
            raise SystemExit(f"not a regular file: {item['path']}")
        data = source.extractfile(member).read()
        if len(data) != item['size'] or hashlib.sha256(data).hexdigest() != item['sha256']:
            raise SystemExit(f"file mismatch: {item['path']}")
        print(f"FILE {item['path']} SHA256 {item['sha256']} SIZE {len(data)}")
        for number, line in enumerate(data.decode('utf-8').splitlines(), 1):
            print(f'{number:4}: {line}')
print(f"VERIFIED_FILES {len(inventory['files'])}")
PYCODE
```
