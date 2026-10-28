# W-02：探查映像的套件文件清點

對應 REQ-060、T-119、T-120 的準備工作。[部分清點](W-02-probe-image-packages.json)記錄三個來源鎖中的 Go／LLVM 探查映像，以及清點工具本身使用的 Python 映像。2026-10-29 每個映像在獨立受限容器執行兩次，輸出逐位元一致，兩份[原始輸出](W-02-probe-images-20261029/)均保留。

| 映像用途 | 平台 | 已安裝套件 | 未記錄 copyright | 單次輸出 SHA-256 |
|---|---|---:|---:|---|
| Go 探查 | linux/amd64 | 78 | 0 | `35db388bd9dcd3229a35d886f93be2549a9a8342dad9fa157e6ef3f92cc2936c` |
| LLVM 探查 | linux/arm64 | 453 | 0 | `d0c65e39a8c34105e79f67165720957ff27be8ba0e31eb383a99ab682272d22f` |
| LLVM 探查 | linux/amd64 | 455 | 0 | `72e4b08cc662b0c178bf47183381ba2462a0b5a41d4aca35fa1cb54d6d67f786` |
| 封存清點工具 | linux/arm64 | 87 | 0 | `f34f0a0450978b0cbd546db46bda2b177d5fa951fc6af5f98fd253be3ff49816` |

## 重跑與身分

[清點腳本](W-02-probe-images-20261029/inventory.sh) SHA-256 為 `dbb9a786bed81d99aa7b54a0fde582cbd24076092f756dc7c7df0666fe03957f`。已取得固定映像時，從專案根目錄以 Go 探查映像為例：

```sh
docker run --rm -i --pull never --platform linux/amd64 --network none \
  --read-only --user 65534:65534 --cap-drop ALL \
  --security-opt no-new-privileges --cpus 1 --memory 512m --pids-limit 64 \
  --tmpfs /tmp:rw,noexec,nosuid,size=32m --entrypoint timeout \
  debian@sha256:89c1ea91e897f0fe0a3731595330bc45b8bdef4047464a07116f891a94054b31 \
  300 sh < docs/implementation/evidence/W-02-probe-images-20261029/inventory.sh
```

其他映像依清點 JSON 的完整映像識別與平台替換。映像拉取不在 300 秒限制內。記錄的執行與清理狀態來自當次操作；原始輸出可逐位元比對。

來源鎖的 Debian 與 arm64 buildpack-deps 是映像索引。[Debian 原始索引](W-02-probe-images-20261029/debian-index.json) SHA-256 與來源鎖一致，其中 linux/amd64 descriptor 為 `sha256:1e1a22175fbf1b32157ddaf0219e1db4e5401fab2b94ed285add64d18587cd2d`。[buildpack 原始索引](W-02-probe-images-20261029/buildpack-index.json) SHA-256 與來源鎖一致，其中 linux/arm64 descriptor 為 `sha256:e95290ae09bb6aff3a9bd67b687a38c450cc256f0779da2ed9926dde4167f483`。這些索引使用 `docker buildx imagetools inspect --raw` 取得；兩個直接指定的平台 manifest 並非索引。

## 限制

腳本僅列舉 dpkg 狀態為 installed 的套件，記錄套件名稱、版本、copyright 路徑、解析後路徑、大小與摘要。路徑限定在 `/usr/share/doc` 或 `/usr/share/common-licenses`，不在範圍內或缺失會標記為未知，不能當成已審查。Python 映像的 `/usr/local` 內容不一定由 dpkg 管理，並未由此清點涵蓋。套件內容、來源、實際分發條件與授權適用性仍待審查；W-02 和產品驗收保持未完成。
