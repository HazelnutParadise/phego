# W-02｜PHP 8.5.12 C 原始檔標記盤點

對應 REQ-060、T-119。固定官方封存檔 SHA-256 `08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209`，在兩次獨立、無網路、唯讀、非 root 且資源受限的 Linux arm64 容器內掃描，輸出逐位元相同。沒有執行來源程式。

共檢查 2,175 個一般 `.c`、`.h` 檔，每檔僅搜尋前 8,192 bytes。PHP License 命中 949、GNU LGPL 83、GNU GPL 8、MIT 23、Apache 1，1,113 檔沒有命中。各標記可在同一檔同時出現，因此數量不可相加推算分類。未命中不代表無授權；其他副檔名、非一般檔、前 8,192 bytes 之外的標記與未納入的文字均不在此檢查範圍。這不是授權分類或散布義務判斷，T-119 仍為 `not_run`。

逐檔路徑、大小、完整內容摘要與命中結果保存在 [壓縮清單](W-02-php-c-headers-20261029.json.gz)。解壓後 SHA-256 為 `922c98686a03aa88b8067673eb147bb5807e01e866b7d2d3f872faf485f8fb79`。掃描程式 SHA-256 為 `854301558b528224507009832f7fed199c652b30d94c51288257b380bdbd7619`。

將 `ARCHIVE` 設為封存檔絕對路徑，以以下命令重跑：

```sh
docker run --rm -i --pull never --platform linux/arm64 --network none --read-only \
  --user 65534:65534 --pids-limit 32 --memory 512m --cpus 1 \
  --cap-drop ALL --security-opt no-new-privileges \
  --mount "type=bind,src=$ARCHIVE,dst=/input/source.tar.xz,readonly" \
  buildpack-deps@sha256:4c9dc8ac4ff58f6fe23d2b007fe3bb8e25716f947adae6fb312d6cfbaac2b3c3 \
  python3 - <<'PYCODE'
import hashlib,json,re,tarfile
from collections import Counter
from pathlib import Path
p=Path('/input/source.tar.xz'); h=hashlib.sha256()
with p.open('rb') as f:
 for b in iter(lambda:f.read(1024*1024),b''): h.update(b)
expected='08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209'
if h.hexdigest()!=expected: raise SystemExit('source digest mismatch')
patterns={
 'spdx_identifier':re.compile(r'SPDX-License-Identifier\s*:',re.I),
 'php_license':re.compile(r'PHP\s+licen[cs]e',re.I),
 'gnu_lgpl':re.compile(r'GNU\s+(?:Lesser|Library)\s+General\s+Public\s+License',re.I),
 'gnu_gpl':re.compile(r'GNU\s+General\s+Public\s+License',re.I),
 'apache_license':re.compile(r'Apache\s+License',re.I),
 'mit_license':re.compile(r'MIT\s+License',re.I),
 'bsd_license':re.compile(r'BSD\s+License',re.I),
}
files=[]; seen=set(); total=0; count=0
with tarfile.open(p,'r:xz') as a:
 for m in a:
  count+=1
  if count>100000: raise SystemExit('archive member limit exceeded')
  name=m.name
  if not (name == 'php-8.5.12' or name.startswith('php-8.5.12/')) or '\\' in name or any(x in ('','.', '..') for x in name.rstrip('/').split('/')): raise SystemExit('unsafe archive member path')
  if name in seen: raise SystemExit('duplicate archive member')
  seen.add(name)
  if not m.isfile() or not name.lower().endswith(('.c','.h')): continue
  if m.size<0 or m.size>32*1024*1024: raise SystemExit('source member size limit exceeded')
  total+=m.size
  if total>512*1024*1024: raise SystemExit('source size limit exceeded')
  f=a.extractfile(m); digest=hashlib.sha256(); prefix=b''; remaining=m.size
  while remaining:
   block=f.read(min(1024*1024,remaining))
   if not block: raise SystemExit('truncated source member')
   if len(prefix)<8192: prefix+=block[:8192-len(prefix)]
   digest.update(block); remaining-=len(block)
  text=' '.join(prefix.decode('utf-8','replace').split())
  markers=[k for k,pattern in patterns.items() if pattern.search(text)]
  files.append({'path':name,'size':m.size,'sha256':digest.hexdigest(),'markers':markers})
files.sort(key=lambda x:x['path']); counts=Counter(k for f in files for k in f['markers']); counts['no_marker']=sum(not f['markers'] for f in files)
print(json.dumps({'schema_version':1,'status':'partial','scope':'regular .c and .h files; marker searches inspect the first 8192 bytes, not a license classification','archive_sha256':expected,'file_count':len(files),'source_bytes':total,'marker_counts':dict(sorted(counts.items())),'files':files},indent=2))
PYCODE
```
