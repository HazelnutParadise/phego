import hashlib, json, os, re, stat
from pathlib import Path
root = Path("/usr/local")
pattern = re.compile(r"LICENSE|LICENCE|COPYING|NOTICE|PATENTS", re.I)
files = []
links = []
count = 0
total_candidate_bytes = 0
def fail_walk(error):
    raise error
for folder, directories, names in os.walk(root, followlinks=False, onerror=fail_walk):
    directories.sort()
    names.sort()
    for name in list(directories):
        p = Path(folder) / name
        if p.is_symlink():
            links.append(str(p))
    for name in names:
        count += 1
        if count > 100000:
            raise ValueError("file count limit exceeded")
        p = Path(folder) / name
        mode = p.lstat().st_mode
        if stat.S_ISLNK(mode):
            if pattern.search(name):
                links.append(str(p))
            continue
        if not stat.S_ISREG(mode) or not pattern.search(name):
            continue
        fd = os.open(p, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, "rb") as handle:
            if not stat.S_ISREG(os.fstat(handle.fileno()).st_mode):
                raise ValueError("candidate is not a regular file")
            data = handle.read(16 * 1024 * 1024 + 1)
        if len(data) > 16 * 1024 * 1024:
            raise ValueError("candidate size limit exceeded")
        total_candidate_bytes += len(data)
        if total_candidate_bytes > 128 * 1024 * 1024:
            raise ValueError("candidate total size limit exceeded")
        try:
            data.decode("utf-8")
            encoding = "utf8_text"
        except UnicodeDecodeError:
            encoding = "binary"
        files.append({"path": str(p), "size": len(data), "sha256": hashlib.sha256(data).hexdigest(), "encoding": encoding})
result = {"schema_version": 1, "status": "partial", "image": "python@sha256:29434a59c93070e470f84663e4289026e04644ea44b5220ef7e07d653318ca1f", "platform": "linux/arm64", "root": str(root), "visited_files": count, "selection": "basename contains LICENSE, LICENCE, COPYING, NOTICE or PATENTS, case insensitive", "files": files, "unfollowed_symlinks": sorted(links), "license_applicability": "unknown"}
print(json.dumps(result, ensure_ascii=False, sort_keys=True, indent=2))
