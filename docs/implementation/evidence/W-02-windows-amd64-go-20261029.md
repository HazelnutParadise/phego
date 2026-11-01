# W-02｜Windows amd64 Go 參考探查

對應 REQ-060、T-119。[GitHub Actions 執行 40857358790](https://github.com/HazelnutParadise/phego/actions/runs/40857358790) 於提交 `e45e5de5f0989508600d908749c7c2d71e00e9a4` 成功，執行環境是 GitHub 代管的 Windows Server 2025 amd64 虛擬機。固定 Go 1.27.3 封存檔的摘要在解壓前核對。

兩次使用各自的來源目錄與建置快取執行 Pure Go 最小程式，皆輸出 `probe go1.27.3 windows/amd64`。兩個程式的 SHA-256 均為 `bd34187eebb4c930947089f703e4e3c87c7fa2f507dc6ec9446f478f7745935d`。此探查沒有從 Go 原始碼重建工具鏈，也沒有執行完整 Go 測試或 Phego 產品測試。T-119 維持 `not_run`。

下列壓縮檔保留 CI 原始輸出，摘要對應解壓後的原始內容：

- [`cpu.txt`](W-02-windows-amd64-go-20261029/cpu.txt.gz)：`9ab44772be5efa082ac28340efbf6179cf6a06fc8e40ad60485348beab3321b0`
- [`input-sha256.txt`](W-02-windows-amd64-go-20261029/input-sha256.txt.gz)：`46eb66eaa84e9f896555f53426e0f233c98ddeaf8dc3c7c9623c725a1a6641ed`
- [`os.txt`](W-02-windows-amd64-go-20261029/os.txt.gz)：`5ad411c6f644b9af8a70751f32ae5513483d74eb1340c2badc5686ca36daac1b`
- [`probe-1.txt`](W-02-windows-amd64-go-20261029/probe-1.txt.gz)：`6e98d3629141451ece4777f4913139b4836ecb7028215fcf4a264a01c135f88f`
- [`probe-2.txt`](W-02-windows-amd64-go-20261029/probe-2.txt.gz)：`6e98d3629141451ece4777f4913139b4836ecb7028215fcf4a264a01c135f88f`
- [`result.txt`](W-02-windows-amd64-go-20261029/result.txt.gz)：`2591d0f4054cca58c48e9ce13ca54c3ec277e8721004df49bd9e64a1fed015e3`
