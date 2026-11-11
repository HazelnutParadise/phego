# W-17：VM 通用執行路徑

需求：REQ-001、REQ-005、REQ-011、REQ-015、REQ-016、REQ-025、REQ-026。局部測試對應 T-009、T-010、T-029～T-032、T-051、T-052。實作進度以 [W-17](../implementation/W-17.md) 為準，本文件記錄設計決定與理由。

本文件供 VM、編譯器與 Runtime 維護者使用。W-17 是第一個能把 PHP 原始碼端對端執行的工作包，建立在 [W-16 的 bytecode 格式](W-16-bytecode.md)、[W-12 陣列](W-12-arrays.md)、W-14 執行框架與 [Runtime 內部配置](runtime-layout.md)之上。

## 開工時的現況（2026-11-11，main 為 d676ee4）

| 部分 | 現況 | W-17 要補的 |
|---|---|---|
| `Engine.Run` | 只回 `UnsupportedError` | 接上編譯器與 VM |
| `Engine.Compile` | 只做語法解析，`Program` 只存語法樹 | 產生經驗證的 bytecode |
| HIR（`internal/ir`） | 結構化的樹，沒有控制流程圖 | 由編譯器走訪產生指令 |
| bytecode 1.0 | 25 個指令、11 個二元運算，沒有一元運算、`!=`、`<=`、陣列寫入、foreach、物件、常數查詢 | 逐批擴充指令 |
| Runtime | execution、heap、value 都是套件私有，以 Go 函式組成動作來測試 | 逐指令的步數與取消檢查、帶檔名行號的錯誤文字、使用者函式 |
| 例外 | 只有 Go 的 `*Throwable{Class, Message}`，沒有 PHP 例外物件 | 第一階段只處理未捕捉的錯誤 |
| 內建函式 | 一個都沒有 | 不在 W-17 範圍，屬於模組工作包 |
| 測試入口 | PHPT 執行器只能跑映像裡的 PHP，沒有 Phego 的 CLI | 讓 Phego 產生可比對的觀測結果 |

## 套件分工

- `internal/vm`：HIR 到 bytecode 的編譯器。它不需要 Runtime，只依賴 `internal/ir` 與 `internal/bytecode`。
- `internal/runtime`：直譯迴圈放在這個套件內。Runtime 的值、heap 與執行框架都是私有型別，另開套件就得匯出整片介面，每個運算還多一層呼叫。規格第 25 章的目錄只是建議，允許調整。
- 根套件：`Engine.Compile` 產生程式，`Engine.Run` 建立請求、執行並把結果轉成 `Result`。

## 分階段交付

W-17 的局部退出條件是「核心程式可執行，錯誤與邊界皆有測試，未支援功能明確回報」。bytecode 目前能表達的 PHP 太少，連 `$i++` 與 `!=` 都沒有，所以第一階段同時把格式擴充到 1.1。

| 階段 | 能執行的 PHP | 編譯時回報不支援的 |
|---|---|---|
| 一 | 純量與字串、所有算術、比較、位元與邏輯運算、型別轉換、`++`／`--`、區域變數、陣列的建立、讀取、單層寫入、`isset`、`unset`、if、迴圈、switch、`exit`、固定位置參數的使用者函式、頂層 `return` | foreach、參照、物件與類別、try／catch、closure、常數、include、eval、內建函式、參數預設值與型別 |
| 之後 | 依上表右欄逐批加入，每批都擴充格式、編譯器、VM 與對照測試 | |

不支援的語法在編譯時回報帶位置的 `UnsupportedError`，不產生會靜默略過的指令。完整產品的範圍不變，階段只決定交付順序。

## bytecode 1.1 新增的指令

格式版本的 minor 由 0 改為 1，1.0 的檔案照樣能載入。操作數規則在 [W-16 文件的指令表](W-16-bytecode.md)。

| 指令 | 用途 |
|---|---|
| Unary | `!`、負號、正號、`~` |
| Cast | 轉成 bool、int、float、string、array |
| IncDec | 對一個位置做前置或後置的遞增、遞減 |
| ArraySet、ArrayAppend、ArrayUnset | `$a[k] = v`、`$a[] = v`、`unset($a[k])` |
| Isset、ArrayIsset | `isset($x)`、`isset($a[k])` |
| Unset | `unset($x)` |
| Exit | `exit`，可帶狀態 |
| Binary 新增 | `%`、`**`、`<<`、`>>`、`!=`、`!==`、`<=`、`<=>`、`xor` |
| Load、ArrayGet 的 Aux | 1 表示安靜讀取，給 `isset`、`??` 使用，不發未定義警告 |

`>` 與 `>=` 沒有自己的運算：PHP 自己也是把兩個運算元交換後用 `<` 與 `<=`，運算元的求值順序由編譯器照原順序放進暫存器來維持。

## 顯示錯誤與記錄錯誤

- 錯誤文字由 VM 產生，包含檔名、行號與 fatal 的 stack trace，必須和 PHP 一字不差。
- 開啟 display_errors 時，文字經輸出層的 `write()` 送出，所以會進入輸出緩衝。`Result.Diagnostics` 是 log_errors 的管道。
- 公開 API 新增 `phego.WithINI(name, value)` 這個 `RunOption`，設定當次執行的 INI 值。這是只增加、不更動既有介面的 Go 嵌入契約變更，專案負責人於 2026-11-11 核准。階段一接受 display_errors、log_errors、html_errors 與 error_reporting，其他名稱回報 `ConfigError`，不靜默忽略。預設值和 `php -n` 相同。

## 安全與並行

- 每個語意步驟先檢查取消與步數，再執行副作用。迴圈回邊、函式進出都有檢查點。
- 驗證器通過不代表每條指令都檢查過：不可達的指令只做操作數形狀檢查，不做資料流檢查。VM 對暫存器、常數、函式與跳躍目標的索引不依賴這點。
- 編譯後的程式可由多個 goroutine 同時執行。暫存器、區域變數、引數堆疊與待處理的完成狀態都屬於當次執行，不放進程式物件。

## 分派、frame、區域變數與取消檢查的布局

每一項都把候選做法放進同一個測試執行檔，跑同樣的五個工作量，先確認輸出、回傳值與步數完全相同，再比時間。工作量是算術迴圈、分支加 `%` 與比較、以 `.` 串接字串、遞迴的 `fib(18)`、陣列填入與讀取。量測在 GitHub 的 ubuntu-24.04（amd64）與 ubuntu-24.04-arm runner 上各交替執行 10 輪，以 benchstat 比較（`tools/benchstat-variants.sh`，「VM layout benchmarks」工作流程，commit 5545d4f）。原始輸出在 [W-17 證據目錄](../implementation/evidence/W-17-vm/benchstat/)。

| 項目 | 採用 | 其他候選在 CI 上的結果 | 理由 |
|---|---|---|---|
| 分派 | 單一 `switch` 迴圈 | 預先解碼成 closure：五個工作量都慢，amd64 慢 11～78%，arm64 慢 11～63%。比較加分支的合併指令：十格中七格量不出差異，其餘三格在 −1.5% 到 +1.3% 之間 | closure 每條指令多一次間接呼叫，還要為每個程式配置 closure。合併指令沒有可量到的好處，不值得多一條執行路徑 |
| frame | 每次執行一塊連續的值陣列，各 frame 取其中一段 | 每次呼叫新配置：`fib` 慢 60%（amd64）與 94%（arm64），其餘量不出差異。每個深度各自重用：`fib` 慢 0.7～1.0%，其餘量不出差異 | 連續陣列配置次數最少，時間不輸其他做法 |
| 區域變數 | frame 自己擁有的位置 | 沿用 `local()` 逐一登記：慢 220%（amd64）與 245%（arm64），配置次數由 23 次變成 8,422 次 | `local()` 每次登記都掃描所有 frame，[Runtime 內部配置](runtime-layout.md)的後續事項已指出這是每次呼叫的主要成本。VM 不再呼叫它，其他呼叫者不受影響 |
| 取消檢查 | 回邊、呼叫、回傳、Checkpoint，以及每 1,024 條直線指令 | 每條指令都檢查：amd64 慢 9～69%，arm64 慢 8～46% | 步數仍然每條指令計量，取消最多延後 1,024 條指令被看見 |

W-16 以合成工作量選了 `switch` 當起點，並要求用真實的值與 Runtime 呼叫重測。這次的結果和當時不同：closure 在 W-16 的 branch-heavy 合成測試比 `switch` 快，在真實的值上則全面較慢。

## 階段一的對照結果

`TestW17PHP8512Programs` 把 `tests/vm/w17/cases` 的 90 個程式交給 Phego 執行，和固定映像中 PHP 8.5.12 的輸出、錯誤文字與退出碼逐位元組比對。每個程式跑三種錯誤模式（只顯示、只記錄、兩者都開），三個 profile 各 270 項。

| profile | 相同 | 回報不支援 | 不同 |
|---|---|---|---|
| 64 位元 arm64 | 261 | 9 | 0 |
| 64 位元 amd64 | 261 | 9 | 0 |
| 32 位元 386 | 261 | 9 | 0 |

回報不支援的 9 項是三個程式各三種模式：`compound_assign.php` 第 61 行與 `compound_assign_undefined.php` 第 34 行對陣列元素做複合指定，`isset_unset.php` 第 22 行做巢狀的 `unset`。它們留在分母裡，等之後的階段補上。

## 已知缺口

- **呼叫深度預設上限為 10,000 層**：PHP 本身沒有上限。直譯器用自己的 frame 堆疊，不會耗盡 Go 的堆疊，但目前沒有記憶體計量，實測 100,000 層約配置 230 MB，所以先設保守的預設值。超過時回報 `ResourceError`，不是 PHP 錯誤。`Limits.Frames` 可以調高。
- **`Limits.Memory` 尚未計量**：Runtime 還沒有邏輯記憶體的帳。
- **字串串接每次都複製整個字串**：迴圈裡用 `.` 累加字串是平方時間。PHP 在沒有其他持有者時會原地附加。
- **編譯期警告以 E_WARNING 過濾**：PHP 用的是 E_COMPILE_WARNING，error_reporting 只關其中一個位元時結果會不同。
- **編譯期 fatal 之前的編譯期警告會遺失**：`Compile` 只回傳 `PHPError`。
- **宣告和內建函式同名的函式**：PHP 在編譯時回報 `Cannot redeclare function`，目前回報不支援，正確的錯誤要補在 `internal/ir`。
- **函式隱含的 return 行號**：使用函式內出現過的最大行號，PHP 使用右大括號那一行。階段一觀察不到差別。
- **`error_reporting` 的 INI 值只接受十進位整數**：PHP 的 `-d` 也接受 `E_ALL` 這類運算式。
