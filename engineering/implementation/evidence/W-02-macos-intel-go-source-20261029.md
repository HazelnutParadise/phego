# W-02｜Intel macOS 的 Go 來源重建

對應 REQ-060、T-119。[GitHub Actions 執行 40871229613](https://github.com/HazelnutParadise/phego/actions/runs/40871229613) 對應提交 `820287e5bc178db61a9d0d0b8a2613c7b438293c` 並通過。執行器為 GitHub 託管的 macOS 15.7.9、Intel amd64；ImageOS 為 `macos15`，ImageVersion 為 `20260824.0482.1`。

[執行流程](../../../.github/workflows/native-macos-intel-go.yml)核對固定 Go 1.27.3 來源及 bootstrap 摘要，在各自的目錄與快取中重建兩次。兩次重建、版本檢查、編譯和執行最小 Pure Go 程式的結束碼皆為 0。兩次 `go` 的 SHA-256 均為 `36f400ba962090e22ae2bd3095caaa8fc572ab49ee79ffc9297274921da42e6b`，兩次測試程式均為 `dbea0e9c4346febe295ad6d7fa5e26a33e758a08c504fe11457eaf7a2f07c9ea`。

每次執行前以實際操作確認沙盒拒絕網路連線與工作目錄外的檔案寫入；系統 Python 在沙盒內回報無法寫入 xcrun 快取，但兩次隔離檢查均回傳 0。這只驗證兩項限制，不能證明沙盒涵蓋所有逃逸途徑。

[第一次建置輸出](W-02-macos-intel-go-source-20261029/build-1.log.gz)、[第二次建置輸出](W-02-macos-intel-go-source-20261029/build-2.log.gz)、結果及環境紀錄皆保存；全部原始檔名稱、大小與解壓後 SHA-256 見[清單](W-02-macos-intel-go-source-20261029/manifest.json)。GitHub artifact 保存期為 30 天。

結果僅涵蓋這一台託管環境、來源和 bootstrap，並只比較 `go` 與最小程式；完整工具鏈、完整 Go 測試和其他 macOS 環境未驗證。這不是 PHP 相容或 Phego 產品驗收，T-119 仍為 `not_run`。
