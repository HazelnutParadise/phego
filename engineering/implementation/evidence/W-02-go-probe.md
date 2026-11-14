# W-02 鎖定 Go 工具鏈的隔離探查

本紀錄對應 [REQ-060](../../data/phego-requirements.json)、局部情境 [T-119](../../data/phego-acceptance-tests.json) 與 [W-02](../W-02.md)。它驗證參考工具鏈的局部操作，不代表 Phego 引擎、完整建置重現性或產品驗收通過。

[工具](../../../tools/reference_go.py) 使用[來源鎖](../../../compat.lock.json)選擇 Go 1.27.3 與精確的 Linux 平台及固定映像。它在啟動容器前核對封存檔摘要，並在無網路、唯讀根目錄、非 root 和有限資源的容器內複製已下載的 Go 工具鏈封存檔，再核對副本摘要、確認 Go 版本與平台，使用 `CGO_ENABLED=0`、`GOPROXY=off`、`GOTOOLCHAIN=local` 編譯與執行最小程式。`HOME` 與建置快取位於容器的暫存空間。本探查使用預編譯的 Go 工具鏈；另一次從來源重建的結果見[來源建置紀錄](W-02-go-source-build.md)。工具不下載映像或來源，重跑前須先取得來源鎖所列檔案及映像。

2026-10-29 在 Mac 的 OrbStack Linux arm64 虛擬機執行下列命令；`GO_ARM64_ARCHIVE` 與 `GO_AMD64_ARCHIVE` 為各自已下載封存檔的絕對路徑：

```sh
python3 tools/reference_go.py "$GO_ARM64_ARCHIVE" --platform linux/arm64 --timeout 300
python3 tools/reference_go.py "$GO_AMD64_ARCHIVE" --platform linux/amd64 --timeout 300
```

兩次回傳 0，工具列出的版本分別為 `go1.27.3 linux/arm64` 和 `go1.27.3 linux/amd64`。編譯所得 `probe` 執行檔的 SHA-256 分別為 `73b78abb52436e5e19d88176d4806d2456948db0d03f1a331d1af93ff3cf63bd` 與 `96c8702847c103a68d37637c41c7f7cc8c260b5b736541dd1728a4c0ef4393ec`。amd64 是在此 Linux 虛擬機中轉譯執行，不代表原生 amd64 或 macOS 測試。工具檔案當次 SHA-256 為 `6e2d61e82b8eff472769c8ba90e96c9768df3132c2618e228547c11b53bcae99`，來源鎖 SHA-256 為 `44e58acca832e60f70eb9ee2ff2760256422c1aa4b7082b141b3c5fce651b144`。

工具清理時輸出的容器 ID 分別為 `4a8caf5a7ee91f6b16f9a43506b36bc6852eae2c542947ea37c59174ef127d52` 與 `1d5da3a262379b290f3f1b205fe62cc4a76acc4aef964f273635ea3e9f7b476b`；對各 ID 執行 `docker inspect` 均回傳 1 和 `no such object`。重跑時應檢查當次輸出的 ID；Docker 無法回應不代表容器已移除。

使用上述版本及 arm64 來源執行 `--timeout 1` 回傳 124，容器 ID 為 `ff9099b0146b450945672f02894f5293c66e7b950ae2e07de292749c42632243`，inspect 回傳 1 和 `no such object`。另一次重跑先以 Docker inspect 確認指定來源掛載的容器 `555d8df9d9fac4ef0a1da27bd26f5d146e543ff7e25aee451b96a21cd5e74958` 處於執行中，再送出 SIGTERM；工具回傳 143，隨後 inspect 回傳 1 和 `no such object`。這不代表所有故障都已實機驗證。本紀錄未附原始執行輸出。

重跑 SIGTERM 探查時，需先確認當次容器已進入 running 狀態，再對工具程序送出 SIGTERM。以下示範只在兩秒後送訊號，不能自行證明容器當時已啟動：

```sh
python3 - "$GO_ARM64_ARCHIVE" <<'PYCODE'
import subprocess, sys, time
process = subprocess.Popen(['python3', 'tools/reference_go.py', sys.argv[1], '--platform', 'linux/arm64', '--timeout', '300'])
time.sleep(2)
process.terminate()
print('exit', process.wait(timeout=30))
PYCODE
```

`python3 -m unittest discover -s tools -p test_reference_go.py` 於本次執行 25 項全部通過，涵蓋鎖定平台、錯誤輸入、容器隔離、來源副本、中斷與清理失敗等行為。測試包含命令結構檢查、受控替身與假 Docker；來源副本動態測試在主機 shell 執行，未啟動 Docker。這些測試不能替代實際工具鏈或跨環境測試。原生 amd64、macOS、Windows 與其他版本仍待驗證。
