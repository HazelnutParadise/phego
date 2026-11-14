# Phego 自己的診斷：rustc 樣式的結構化錯誤

追蹤：REQ-101、T-201、T-202（`planned`／`not_run`）。對應 AGENTS.md 的 Diagnostics 段落。

## 範圍

只涵蓋 Phego 自己的錯誤：設定與執行選項、policy 規格、bytecode 載入與驗證，之後擴及其他錯誤型別和 CLI。PHP 規定的輸出（warning、fatal error、例外訊息、`getMessage()`、`getCode()`、行號與順序）一律維持和 PHP 8.5.12 一字不差，不帶 PHG 代碼，也不經過這套渲染器。

## 設計

- **公開子套件 `phego/diag`**：`Diagnostic`、`Span`、`Severity`、`Code` 與 `Render`。根套件的 `phego.Diagnostic` 早已是 PHP notice／warning 的型別（`Result.Diagnostics`），所以不在根套件新增同名型別，也不用 alias 到 internal 型別（`go doc` 只會顯示 alias，看不到欄位）。
- **單位用 byte**：`Column`、`Offset`、`Length` 都以 byte 計，和 `go/token` 相同，遇到非法 UTF-8 也有定義。渲染器從原始碼文字換算成終端欄寬：tab 展開成 4 格，中日韓全形字算 2 格。只有 `Offset` 沒有 `Column` 時，拿得到原始碼就自行算出欄位，所以 bytecode 錯誤也能畫出標記。
- **代碼只有一個來源**：公開套件 `phego/diag/codes` 為每個代碼匯出一個常數，宿主可以寫 `d.Code == codes.MountPath` 比對，不必手打字串；`codes.Lookup` 取標題、note 與 help，`codes.All` 列出全部代碼。底層 registry 不匯出，宿主無法改掉 Phego 回報的內容。registry 以常數當 map key，重複代碼會直接編譯失敗。`engineering/diagnostics/codes.md` 由 `go generate ./diag/codes` 產生，測試比對兩者是否一致，也禁止 `PHG` 字面值出現在 registry 以外的正式程式碼。代碼發佈後不改號、不重用，停用時標成 retired。`PHG1xxx` 是設定與執行選項，`PHG2xxx` 是 bytecode。
- **note 和 help 綁在代碼上**：每個代碼在 registry 寫一次原因與修法，建構點只填 `Code`，動態值放在 `Reason` 或 `Detail`。help 是純文字，不會被自動套用。
- **既有錯誤只新增**：`ConfigError` 新增匯出欄位 `Code` 與方法 `Diagnostic()`，`Error()` 仍是原本的單行。宿主自己建立、沒有 `Code` 的 `ConfigError` 回報 `PHG1000`。測試掃描根套件，確保引擎建立的每個 `ConfigError` 都有代碼。`phego.Diagnose(err)` 沿錯誤鏈找出第一個能描述自己的錯誤，nil 和其他錯誤回傳 false。
- **相容性備註**：`ConfigError` 新增欄位後，宿主若用不具名欄位的複合字面值（`ConfigError{"a", "b"}`）會編譯失敗。Go 1 相容承諾允許新增欄位，`go vet` 也會對這種寫法警告。宿主可以填自己的代碼，不在代碼表裡的代碼會保留，note 與 help 用 PHG1000 的內容。
- **bytecode**：`Error` 新增 `Source` 與 `Pos`，`Build` 失敗時補上出錯指令的原始碼位置。位置和驗證共用同一組檢查，驗證會拒絕的位置不會出現在診斷裡。`bytecode.Diagnose` 也處理直接回傳的 sentinel 錯誤，例如 `ErrVersion`。
- **安全與上限**：渲染器把輸入裡的控制字元、雙向文字控制字元（例如 U+202E）與非法 UTF-8 換成 U+FFFD，原始碼和訊息不能操控終端。行依 PHP 的規則切分，`\n`、`\r\n` 與單獨的 `\r` 都算一行。長行只顯示主要 span 附近 160 欄，視窗外的次要 span 改列成 note，次要 span 最多畫 16 個，其餘以 note 註明省略數量。原始碼由宿主透過 `Options.Source` 提供，遮蔽敏感內容是提供者的責任。
- **顏色**：只有 `Options.Color` 開關。函式庫不知道輸出會寫到終端、log 還是 HTTP 回應，所以 `NO_COLOR` 與 isatty 偵測交給 CLI（W-35）。
- **機器可讀**：`Diagnostic` 的 JSON 和終端格式帶著相同的代碼、位置、note 與 help，`Severity` 以 `"error"`／`"warning"` 表示。

## 遷移計畫

| 錯誤 | 現況 | 下一步 |
|---|---|---|
| `ConfigError`、policy `SpecError` | 已有代碼與 `Diagnostic()` | 完成 |
| bytecode `Error` 與 sentinel | 依錯誤種類給 7 個代碼，Build 帶原始碼位置 | 把約 80 個 `Detail` 字串改成各自的代碼常數，Load 也補上 byte 範圍 |
| `LimitError`、`ResourceError` | 只有 `Error()` | 新增 `Code` 與 `Diagnostic()`，位置是 limit 名稱，note 寫上限與用量 |
| `PolicyError` | 只有 `Error()` | 依 `Rule`（not-granted、outside-root 等）給代碼，Target 作為主要位置 |
| `HostError`、`ReentrancyError`、`UnsupportedError` | 只有 `Error()` | 給代碼，位置是 host function 名稱或功能名稱 |
| `PHPError` | 保持 PHP 文字 | 不加 PHG 代碼。等 VM 接上後，另外提供明確開啟的開發者檢視，附上原始碼標記，PHP 看到的輸出不變 |
| `tools/*.py` 的 CLI 錯誤 | 純文字 | 改用同一份代碼表，輸出 rustc 樣式並支援 JSON |
| W-09 剖析錯誤 | PHP 文字 | 保持 PHP 文字，開發者檢視同 `PHPError` |
