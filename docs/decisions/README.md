# Architecture decisions

This register is synchronized with chapter 28 of the full specification. The decision text is in Traditional Chinese. Confirmed product decisions are distinguished from design baselines and experimental gates.

## Decision register

以下紀錄區分使用者已確認方向與本版設計基準。未將所有工程細節誤記為已獲使用者逐項核准。

| 編號／狀態 | 決策 | 未採用的選項與理由 |
|---|---|---|
| ADR-01 已確認 | 專案名稱 Phego，repo 名稱 phego | 其他命名未採用；不因此宣稱商標已完成審查 |
| ADR-02 已確認 | Go 為核心實作語言 | Rust 不作主要核心，因產品需要 Go 原生嵌入與整合 |
| ADR-03 已確認 | Go 呼叫一個函式執行 PHP | PHP 轉 Go 原始碼不列為達成嵌入需求的必要途徑 |
| ADR-04 已確認 | 依 Go 特性重新設計內部 | 不以實作成本作理由照抄 Zend 或現有第三方 VM |
| ADR-05 已確認 | goroutine 並行處理獨立請求 | 不把 PHP-FPM 多程序工作池當一般模式的核心 |
| ADR-06 已確認 | 保留 Apache／Nginx，支援 FastCGI | 不要求既有網站改用內建 HTTP 伺服器 |
| ADR-07 本版基準 | 完整 VM 路徑與可選 LLVM 後端 | 不以 LLVM 作核心必需品，不自行重寫通用機器碼產生器 |
| ADR-08 本版基準 | 高階語意 IR、效果資訊與 SSA | 不把 Zend Opcode 當公開或固定內部格式 |
| ADR-09 實驗閘門 | 暫存器式 VM 作起始比較基準 | 不預設堆疊式或暫存器式在所有工作負載都更快 |
| ADR-10 本版基準 | Go GC 管實體記憶體，PHP 邏輯生命週期獨立 | 不以 Go finalizer 模擬全部 PHP 解構語意 |
| ADR-11 本版基準 | Engine／Program 可共用，Execution 隔離 | 不共用請求的 static、INI、輸出與錯誤狀態 |
| ADR-12 本版基準 | 原生區段使用受控 handle ABI | 不直接操作 Go 私有 ABI 與任意 Go heap 指標 |
| ADR-13 本版基準 | 能力與資料來源追蹤分層 | 不用關鍵字黑名單宣稱完整阻擋注入 |
| ADR-14 本版基準 | 高風險執行使用程序隔離 | 不把 goroutine 稱為硬性安全沙箱 |
| ADR-15 本版基準 | 單檔包含必要 Runtime，動態程式保留執行能力 | 不把產生機器碼解讀為 Runtime 可以完全消失 |
| ADR-16 已被 ADR-23 取代 | 開發參考版本可先鎖定現代 PHP；完整認證涵蓋第一版起的全部公開發行 | 不再用 PHP 8.5／64 位元縮小完整產品範圍 |
| ADR-17 本版基準 | 有界固定接納作基準，自適應策略另驗證 | 不預設依負載自動調整一定較穩定 |
| ADR-18 本版基準 | 完整測試分母與功能級能力表 | 不以大量跳過測試包裝成高相容率 |
| ADR-19 本版基準 | Go VM 核心不依賴 Zend 正式回退 | 不在不支援路徑暗中執行官方 PHP |
| ADR-20 本版基準 | 先共用乾淨緩衝，完整 Execution 池化另驗證 | 不因配置成本先承擔跨請求殘留風險 |
| ADR-21 本版基準 | 安全修補高於複製漏洞行為 | 不把未修補的錯誤當作必須保留的舊版語意 |
| ADR-22 本版基準 | 以工作包與驗收決定完成 | 不預先宣稱效能倍數、全網站支援或固定完工日 |
| ADR-23 已確認 | 全部 PHP 發行追溯至第一版，版本與平台行為精確可選 | 取代 ADR-16 的現代版範圍限制；不以來源難找或實作成本排除舊版 |
| ADR-24 已確認 | 效能目標超越全部現有可比 PHP 編譯器／引擎 | 不以只比 Zend、只看平均或跳過不利競品達標 |
| ADR-25 已確認 | 真實專案強制驗收，包含 WordPress、Laravel 與實際應用 | 不以首頁、空框架、單元測試或合成 benchmark 代替整站 |
| ADR-26 已確認 | 指定 HazelnutParadise/phego 與 MIT | 不擅改 owner／授權，不將第三方 fixture 改標 MIT |
| ADR-27 已確認 | 英文 README；The safest PHP in the world | 以「PHP 是世界上最好的語言」迷因為梗，slogan 不當成既有安全認證，成果以實際證據表示 |
| ADR-28 本版基準 | 全目標逐格驗收，未知／缺失／平手不可當成功 | 分階段發布可以進行，但完整交付條件不能被縮減 |
