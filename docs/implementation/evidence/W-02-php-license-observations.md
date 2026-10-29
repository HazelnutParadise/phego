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

## bcmath 原始檔的授權標記

2026-10-29 對同一封存檔的 `ext/bcmath/libbcmath/` 下 28 個一般 `.c`／`.h` 檔案，檢查各檔前 4096 bytes。結果及各檔完整內容摘要記錄於 [標記清單](W-02-bcmath-headers.json)，SHA-256 為 `eded231667ca8efac8415e485624b6d3ba12c97b431b244b31345683ed8d3542`。兩次獨立受限容器執行的輸出逐位元相同。

21 個檔案的標頭文字指定 GNU Lesser General Public License 第 2 版或更新版本，5 個標示 PHP License 3.01，`xsse.h` 標示 MIT License，`config.h` 沒有命中這三種文字。目錄內的 `LICENSE` 文件是 LGPL 2.1 通用文字，不能把該檔名自動套用到目錄內每個檔案。未命中不代表無授權，這些字串比對也不是完整授權分類或分發義務判斷。其他檔案、依賴及實際產物的適用條件仍待審查。

同目錄的 [README](https://github.com/php/php-src/blob/136e049ac10b032c83018e65b63b6df858943750/ext/bcmath/libbcmath/README.md) 將 bcmath library 描述為 LGPL 2.1，並記錄它是原始 bcmath 的 PHP fork。該文件在固定來源封存檔與固定 Git 提交的 SHA-256 均為 `b8280ba681743c519e8c2d7314a0b4321505323d833e5ed5e028af9cfffe43cd`。這份通用描述與個別檔案標頭需一併審查，不能據此單獨判定 `config.h` 的條件。

重跑時將 `ARCHIVE` 設為固定來源封存檔的絕對路徑；指令檢查封存檔摘要後只讀取資料，不執行來源。輸出與上述清單比對需逐位元一致。

```sh
docker run --rm -i --pull never --platform linux/arm64 --network none --read-only \
  --user 65534:65534 --pids-limit 32 --memory 512m --cpus 1 \
  --cap-drop ALL --security-opt no-new-privileges \
  --mount "type=bind,src=$ARCHIVE,dst=/input/source.tar.xz,readonly" \
  buildpack-deps@sha256:4c9dc8ac4ff58f6fe23d2b007fe3bb8e25716f947adae6fb312d6cfbaac2b3c3 \
  python3 - <<'PYCODE'
import hashlib,json,re,tarfile
from pathlib import Path
p=Path('/input/source.tar.xz')
h=hashlib.sha256()
with p.open('rb') as stream:
 for block in iter(lambda: stream.read(1024*1024),b''): h.update(block)
expected='08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209'
if h.hexdigest()!=expected: raise SystemExit('source digest mismatch')
files=[]
with tarfile.open(p,'r:xz') as a:
 for m in a:
  if not m.isfile() or not m.name.startswith('php-8.5.12/ext/bcmath/libbcmath/') or not m.name.endswith(('.c','.h')): continue
  f=a.extractfile(m); data=f.read(); prefix=' '.join(data[:4096].decode('utf-8','replace').split()); markers=[]
  if re.search(r'GNU Lesser General Public License.*?version 2 of the License.*?any later version',prefix,re.I): markers.append('LGPL version 2 or later text')
  if 'version 3.01 of the PHP license' in prefix: markers.append('PHP license 3.01 text')
  if 'MIT License' in prefix: markers.append('MIT License text')
  files.append({'path':m.name,'size':len(data),'sha256':hashlib.sha256(data).hexdigest(),'markers':markers})
files.sort(key=lambda x:x['path'])
if len(files)!=28 or len({x['path'] for x in files})!=28: raise SystemExit('unexpected source inventory')
print(json.dumps({'scope':'first 4096 bytes of regular .c/.h files under ext/bcmath/libbcmath; marker matches only, not license classification','archive_sha256':expected,'status':'partial','files':files},indent=2))
PYCODE
```
