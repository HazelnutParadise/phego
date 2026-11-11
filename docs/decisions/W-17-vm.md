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

## 待補

分派方式、frame 與區域變數的布局要用 benchstat 比較多種做法後才定案，結果與選擇理由會補在本文件。
