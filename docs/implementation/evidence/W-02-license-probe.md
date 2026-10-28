# W-02 來源授權文件探查

此紀錄對應 [REQ-060 祕密與重現](../../data/phego-requirements.json)及局部情境 [T-119](../../data/phego-acceptance-tests.json)，屬於 [W-02](../W-02.md)。它是參考來源的文件清點，不是完整依賴授權審查，也不是發行許可結論。Phego 原創部分維持根目錄 MIT 授權。

已核對 PHP 8.5.12 官方來源封存檔 SHA-256 `08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209`，再以檔名篩選封存檔內的一般檔案。篩到的 17 個檔案路徑、大小與各自摘要列於 [PHP 授權文件清點](W-02-php-license-files.json)。清點不執行檔案，也不將來源複製進專案。檔名以大小寫不敏感方式比對 `license`、`licence`、`copying` 或 `notice`；字詞兩側須為檔名邊界或 `.`, `_`, `-`。

PHP 主檔 `LICENSE` 標示 PHP License 3.01，但封存內容不只一種授權。例如 `ext/date/lib/LICENSE.rst` 標示 MIT，`ext/lexbor/LICENSE` 與 `NOTICE` 標示 Apache License 2.0 及其通知。各檔的適用範圍、來源標頭、外部套件與發行條件仍須逐項審查，不能只依檔名判斷。

[來源鎖](../../../compat.lock.json)記錄 Go 1.27.3 Linux arm64 官方封存檔的取得位置及摘要。該封存檔中的 `go/LICENSE` 摘要為 `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad`，`go/PATENTS` 摘要為 `96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc`。LLVM 23.1.4 固定提交上游原始檔 [llvm/LICENSE.TXT](https://github.com/llvm/llvm-project/blob/7a941dfb2877a0b8cf30323fefa85ad019e0b0f5/llvm/LICENSE.TXT) 摘要為 `8d85c1057d742e597985c7d4e6320b015a9139385cff4cbae06ffc0ebe89afee`，標示 Apache License 2.0 with LLVM Exceptions。來源鎖中的 `license` 欄位只描述已核對的主要授權文件，不代表整份封存或其依賴都只有該授權。

重跑 PHP 清點時，將 `ARCHIVE` 設為已下載的 PHP 來源封存檔路徑，先按來源鎖核對其摘要，再執行以下唯讀命令：

```sh
python3 - "$ARCHIVE" <<'PYCODE'
import hashlib, re, sys, tarfile
pattern = re.compile(r'(?:^|[._-])(?:license|licence|copying|notice)(?:[._-]|$)', re.I)
with tarfile.open(sys.argv[1], 'r:xz') as archive:
    for entry in archive:
        if entry.isfile() and pattern.search(entry.name.rsplit('/', 1)[-1]):
            content = archive.extractfile(entry).read()
            print(entry.name, len(content), hashlib.sha256(content).hexdigest())
PYCODE
```

將 `GO_ARCHIVE` 設為來源鎖中已核對摘要的 Linux arm64 Go 封存檔路徑，可用以下命令重查兩份文件；讀取失敗或摘要不符會回傳非零：

```sh
python3 - "$GO_ARCHIVE" <<'PYCODE'
import hashlib, sys, tarfile
expected = {
    'go/LICENSE': '911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad',
    'go/PATENTS': '96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc',
}
with tarfile.open(sys.argv[1], 'r:gz') as archive:
    for name, digest in expected.items():
        data = archive.extractfile(name).read()
        if hashlib.sha256(data).hexdigest() != digest:
            raise SystemExit(f'{name}: digest mismatch')
        print(f'{name}: match')
PYCODE
```

LLVM 記錄的是固定提交的原始檔，並非其二進位封存檔的完整清點。下載[固定提交的原始授權檔](https://raw.githubusercontent.com/llvm/llvm-project/7a941dfb2877a0b8cf30323fefa85ad019e0b0f5/llvm/LICENSE.TXT)後，可用 `sha256sum "$LLVM_LICENSE"` 核對；`LLVM_LICENSE` 是下載後的檔案路徑。

## 歷史來源的授權線索

官方 museum 的 `php-108` 來源封存檔中，`version.h` 標示 1.0.8，其標頭聲明 GPL 第 2 版或更新版本。這與現代 PHP 的主要授權不同；來源內容是否可納入專案及其分發義務需要另行審查。實際封存檔與文件摘要、隔離檢查範圍及未驗證事項記錄於 [W-49 歷史來源線索](W-49-php-108-source.json)。該封存檔不能代表第一個公開版本，也沒有執行歷史引擎。

## 參考建置映像的套件文件

針對來源鎖中的參考建置映像索引，分別以 `linux/arm64` 與 `linux/amd64` 啟動隔離容器。兩者各有 413 個已安裝的 dpkg 套件，均找到 `/usr/share/doc/<套件>/copyright`；各套件版本、文件路徑和摘要列於[映像套件清點](W-02-build-image-packages.json)。清點只記錄檔案摘要，不表示已審查條款、來源取得、實際建置包含哪些套件或分發義務，也不代表 Phego 執行時依賴這些套件。

可在已取得來源鎖固定映像的環境，以 `PLATFORM=linux/arm64` 或 `PLATFORM=linux/amd64` 重跑以下唯讀清點：

```sh
docker run --rm -i --pull never --network none --read-only --user 65534:65534 \
  --platform "$PLATFORM" \
  buildpack-deps@sha256:2d7fd9aab90c5444e6c4f5c59fce9ab6ba6bce7b8709ef74fcd1f618cd2e867b python3 - <<'PYCODE'
import hashlib, pathlib, subprocess
lines = subprocess.check_output(['dpkg-query', '-W', '-f=${Package}\t${Version}\n'], text=True).splitlines()
for line in lines:
    package, version = line.split('\t', 1)
    path = pathlib.Path('/usr/share/doc') / package / 'copyright'
    print(package, version, path, hashlib.sha256(path.read_bytes()).hexdigest(), sep='\t')
PYCODE
```

逐檔適用範圍、來源標頭、外部依賴、其他版本來源及容器套件的授權條款仍待審查；實際產物所需歸屬與分發條件也尚未判定。完整結果須在 W-02 退出前補齊，不以本檔宣稱通過。
