# W-02｜Windows amd64 的 Go 來源重建

對應 REQ-060、T-119。[GitHub Actions 執行 40868256713](https://github.com/HazelnutParadise/phego/actions/runs/40868256713) 對應提交 `6b3c6f3360deb27de17e733c9786750fdeb624d2`，工作結果為 success。這是 GitHub 代管的 Windows Server 2025 amd64 虛擬機，runner 的 ImageOS 為 `win25-vs2026`，ImageVersion 為 `20261021.278.1`。

[執行流程](../../../.github/workflows/native-windows-go-source.yml)核對來源鎖中的 Go 1.27.3 來源封存檔及 Windows amd64 bootstrap 摘要，在兩個分開的目錄與快取中各自執行 `make.bat`。兩次來源重建及以新建工具編譯、執行最小 Pure Go 程式的結束碼均為 0。兩次 `go.exe` 的 SHA-256 均為 `61c240cc258a988cefaa1ed78be817c7863af5ded51df694685c116b71d65c34`；測試程式均為 `f4c053e7e34db70ce7faba613119132208928cba6738683ee40d379f4c91607c`。

[第一次建置輸出](W-02-windows-go-source-20261029/build-1.log.gz)和[第二次輸出](W-02-windows-go-source-20261029/build-2.log.gz)、各次結束碼、測試輸出、平台及來源鎖摘要均以壓縮檔保存。全部原始檔案名稱、大小與解壓後 SHA-256 列於[清單](W-02-windows-go-source-20261029/manifest.json)。原始 GitHub artifact 保存期為 30 天。

這項結果只涵蓋單一 Windows amd64 虛擬機、Go 版本、來源與 bootstrap，且只比較 `go.exe` 和最小程式；沒有驗證整套工具鏈、完整 Go 測試或其他 Windows 環境。工作流程停用 Go 模組代理，但未在作業系統層禁止網路。這不是 Phego 產品驗收，T-119 仍為 `not_run`。
