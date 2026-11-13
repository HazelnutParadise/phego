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
| 二 | 陣列元素的複合指定、`??=`、`++`／`--`，任意層的巢狀寫入與 `unset`，`foreach` 傳值，參數預設值，讀寫字串位移 | foreach 傳參照、解構、走訪物件，透過非變數寫入陣列，參照，參數型別、傳參照與不定參數，具名引數，以常數為預設值，PHP 不 intern 的字串預設值 |
| 三 | 參照指定 `$a = &$b`（變數與任意層元素都可以當來源或目標），`foreach` 傳參照，傳參照的參數（含預設值） | 解構、走訪物件，參照到函式呼叫結果或屬性，把不是變數的值傳給傳參照參數，兩個元素之間參照指定的運算式值，`global`、`static`，陣列字面值裡的 `&$x`，回傳參照，參數型別與不定參數，具名引數 |
| 之後 | 依上表右欄逐批加入，每批都擴充格式、編譯器、VM 與對照測試 | |

不支援的語法在編譯時回報帶位置的 `UnsupportedError`，不產生會靜默略過的指令。完整產品的範圍不變，階段只決定交付順序。

## bytecode 1.1 新增的指令

格式版本的 minor 由 0 改為 1，1.0 的檔案照樣能載入。操作數規則在 [W-16 文件的指令表](W-16-bytecode.md)。

| 指令 | 用途 |
|---|---|
| Unary | `!`、負號、正號、`~` |
| Cast | 轉成 bool、int、float、string、array |
| IncDec | 對一個位置做前置或後置的遞增、遞減 |
| ArraySet、ArrayAppend、ArrayUnset | `$a[k] = v`、`$a[] = v`、`unset($a[k])`。ArraySet 與 ArrayAppend 的 Aux 可標示鍵或值是變數位置，由指令在容器檢查之後自己讀取 |
| ArrayAdd | 對陣列字面值加一個元素，鍵的規則照 PHP 的字面值：未定義變數當鍵只發未定義警告 |
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

## 階段二

格式升到 1.2，新增 FetchDim、IterInit、IterNext、IterKey 與 ParamDefault。1.0 與 1.1 的檔案照樣能載入，其中出現 1.2 指令時以 `ErrInvalid` 拒絕。操作數規則在 [W-16 文件](W-16-bytecode.md)。

- **元素寫入**照 PHP 的取值順序：先檢查容器，再讀鍵、轉換鍵、讀值、寫入。元素位置只能用在編譯器產生的那一段直線指令裡，驗證器會拒絕其他用法，所以手寫的 bytecode 也無法經由過期的位置寫入。
- **`foreach` 傳值**走訪迴圈開始時的陣列，靠寫入時複製保留，不提前複製。走訪狀態放在暫存器，屬於當次執行，每一圈都檢查取消。
- **參數預設值**照 PHP 的快取：同一次執行中，不是共享計數型別的預設值只在第一次省略引數時求值，裡面的警告只印一次。快取屬於當次執行，不放進共用的程式物件。字串是否被 intern 依各字面值與運算子的結果型別判斷，PHP 不 intern 的寫法一律在 `Compile` 拒絕，不在執行期猜測。這條規則是在固定映像量 50 種寫法歸納出來的。
- **字串位移**的補空白與複製分塊進行並檢查取消。延長字串的寫入接在緩衝區後面，20 萬次寫入由 1.1 秒、20.7 GB 降到 31～41 毫秒、0.5 MB。

測試程式加到 192 個，三個 profile 各 576 項全部和 PHP 8.5.12 逐位元組相同。階段一回報不支援的三個程式也通過了。兩位獨立審查者分別跑了 611 個 PHP 行為探測與惡意 bytecode、取消、隔離、格式相容的檢查，沒有 P0 或 P1。另以 3,000 個隨機程式對照 PHP，0 個不同。

## 階段三

格式升到 1.3，新增 Bind 與 MakeRef，IterInit 與 IterNext 加上 IterByRef，FetchDim 加上 FetchForRef 用途。1.0 到 1.2 的檔案照樣能載入，其中出現 1.3 的指令時以 `ErrInvalid` 拒絕。操作數規則在 [W-16 文件](W-16-bytecode.md)。

- **參照指定**照 zend_compile_assign_ref 的順序：先算目標的 key，再取來源，最後才取目標的元素。兩邊都不只是變數時，取完來源就先把它變成參照（MakeRef），再去取目標，這樣目標的寫入讓陣列搬家時，手上也不會留著指向舊位置的指標（PHP bug #71539 的修法）。元素是用來當參照的那一層，字串位移的錯誤是 `Cannot create references to/from string offsets`。來源是未定義的變數或元素時，參照裡放的是 null，不發警告。
- **傳參照的參數**在呼叫端照 SEND_REF，每個引數在它自己的位置變成參照，所以後面的引數改到同一個變數時，函式看到的是改過的值。被呼叫端以 Bind 接收；省略引數時預設值照樣進入每次執行的預設值快取。
- **foreach 傳參照**照 FE_RESET_RW：主體是陣列時迴圈自己握有一份參照，主體是未定義變數時發出警告、不建立變數，不是陣列時不變成參照。每一圈照 FE_FETCH_RW 跟著變數目前的陣列走，迴圈中途換掉陣列、刪除或加入元素，都和 heap.foreachRef（W-12 依 PHP 8.5.12 實作的走訪）結果相同，走訪位置記在陣列上，迴圈結束或函式返回時清掉。值的目標是元素時，先取出元素的參照，再取目標元素綁上去。
- **暫存器裡的參照**只有 MakeRef 和傳參照的 IterNext 會產生，下一個 Push 或 Bind 就用掉，驗證器也不讓 Bind 之後再讀同一個暫存器，參照計數不會因為暫存器多留一份而改變陣列複製時的行為。

獨立審查跑了約 90 個 PHP 探測程式，沒有 P0 或 P1，找到的四個問題都已修正並加上修正前會失敗的回歸測試：

- `foreach ($a[] as &$v)` 原本在 `Compile` 回報 PHP 的 `Cannot use [] for reading`。PHP 對傳參照 foreach 的變數主體以寫入編譯（zend_compile_foreach 的 BP_VAR_W），所以會新增 `$a[0] = null` 再警告主體不是陣列；傳值的 foreach 仍是那個 fatal。
- 傳參照引數的參照原本在 Push 後仍留在暫存器。一般函式靠 liveness 釋放，大到跳過 liveness 的函式（暫存器乘指令數超過 vmLivenessWords）會多一個持有者，陣列複製時共用了 PHP 會複製的元素。現在 Push 把暫存器裡的參照移交給引數堆疊。
- 迭代器暫存器原本只靠槽位編號找到走訪狀態，手寫 bytecode 留下的複本會在槽位重用後走訪別的迴圈；以 Const 等指令覆寫迭代器暫存器時，走訪狀態也不會釋放。現在狀態比對迴圈的參照與每次重用遞增的世代，所有覆寫暫存器的路徑都經過會結束走訪的釋放。
- CallValue 原本是唯一不解開暫存器裡參照的指令。

複查確認四項都已修正，另外找到一個 P3：同一個走訪槽位重用 2^31 次之後，世代編號讓識別碼變成負數，正常的迴圈也會被拒絕。現在以無號比較，世代將要繞回的槽位直接停用，不再重用。

測試程式加到 220 個，三個 profile 各 660 項全部和 PHP 8.5.12 逐位元組相同。

## 階段一的對照結果

`TestW17PHP8512Programs` 把 `tests/vm/w17/cases` 的 90 個程式交給 Phego 執行，和固定映像中 PHP 8.5.12 的輸出、錯誤文字與退出碼逐位元組比對。每個程式跑三種錯誤模式（只顯示、只記錄、兩者都開），三個 profile 各 270 項。

| profile | 相同 | 回報不支援 | 不同 |
|---|---|---|---|
| 64 位元 arm64 | 261 | 9 | 0 |
| 64 位元 amd64 | 261 | 9 | 0 |
| 32 位元 386 | 261 | 9 | 0 |

回報不支援的 9 項是三個程式各三種模式：`compound_assign.php` 第 61 行與 `compound_assign_undefined.php` 第 34 行對陣列元素做複合指定，`isset_unset.php` 第 22 行做巢狀的 `unset`。它們留在分母裡，等之後的階段補上。

## 獨立審查後的修正

三位獨立審查者檢查了 bb8e49a，共跑 416 個端對端探測。確認的問題都以 PHP 8.5.12 固定映像的實際輸出為準修正，每項有回歸測試（`w17_hardening_test.go`、`internal/runtime/w17_hardening_test.go`、`w17_review_probes_test.go`、`internal/vm/review_test.go`）。

**PHP 原始碼不能讓宿主程序崩潰。** 長運算式的 lowering、陣列的釋放與比較改成不靠 Go 堆疊遞迴。超過上限時回報 PHP 自己的錯誤：

| 情況 | Phego 的上限 | PHP 8.5.12 在 linux/arm64 映像的實測 |
|---|---|---|
| 運算式巢狀（含 `1+1+…` 這類鏈），`ir.MaxExpressionNesting` | 100,000，超過回編譯錯誤 `Maximum call stack size of 8339456 bytes … reached during compilation. Try splitting expression` | `+` 鏈 55,000 項可編譯，60,000 項起同一個 fatal。`$a[0]…` 鏈 30,000 可以，50,000 fatal |
| 陣列比較的巢狀，`runtime.MaxComparisonNesting` | 100,000，超過丟出 `Error: Maximum call stack size reached during comparison` | `==` 30,000 層可以，40,000 層起丟出。`===` 60,000 可以，70,000 起丟出 |
| 回傳值匯出 | 256 層、1,048,576 個元素，超過回 `ResourceError` | 不適用（宿主介面） |

Phego 的上限都高於 PHP 的門檻，所以 PHP 能編譯或比較的程式 Phego 也能。PHP 的門檻取決於平台的堆疊大小，介於兩者之間的程式 PHP 會失敗而 Phego 會成功。PHP 自己在 300,000 層的 `unset` 會 segfault，Phego 能正常釋放。

**限制的實際行為和文件一致。**

- `Limits.Memory` 大於 0 時 `Run` 回報 `UnsupportedError`，直到記憶體計量完成。
- `Limits.Diagnostics` 只限制紀錄的筆數，超過後不再記錄，執行繼續，顯示不受影響。
- `Limits.Frames` 為 0 表示不設上限，專案負責人於 2026-11-12 決定照 PHP。
- `Engine.Close` 取消進行中的編譯與執行。`Compile` 在解析與 lowering 中檢查 context。
- 內部錯誤是 `*InternalError`（PHG9000）。清理期限從拆除開始計時，不是從執行開始。

**PHP 可見行為的修正。** 字串插值兩段時延後讀取、三段以上逐段轉字串。元素寫入的順序是容器檢查、讀鍵、轉換鍵、讀值、寫入。單獨一個變數當陳述式不發警告。`??=` 右邊的單純變數用目標那一行。宣告和內建函式同名的函式回報 PHP 的 `Cannot redeclare function`。

**Codex 審查後的修正。** 名稱比對只折疊 ASCII 字母（`internal/phpname`），`É()` 與 `é()` 是兩個函式，前端、HIR、編譯器與 VM 共用同一個規則。只用 `\r` 換行的檔案行數和語法分析器一致。`unset($u[$k])` 先警告容器再讀鍵。排隊等名額的執行遇到 `Close` 以取消結束。大字串串接每 4 MiB、大型共用陣列的複製每 65,536 個元素檢查一次取消。`RunFile` 在開檔前拒絕無法強制的記憶體上限。

**被淘汰的比較候選已移出正式程式碼。** closure 與合併指令只留在測試檔。frame、區域變數與取消檢查的其他候選需要改動直譯器內部，已刪除，上表的量測結果保留為證據。

## 已知缺口

- **無窮遞迴在預設設定下沒有東西會擋**：PHP 靠 memory_limit（`php -n` 為 128 MB）以 `Allowed memory size exhausted` 結束。Phego 還沒有記憶體計量，實測每層約 1.1～1.5 KB（100,000 層 183 MB，1,000,000 層 1.52 GB），會一路成長到宿主記憶體用完。只有 `Limits.Steps`、`Limits.Frames`、context 或 `Close` 能停止。
- **`Limits.Memory` 尚未計量**：Runtime 還沒有邏輯記憶體的帳，設定它會被拒絕。
- **編譯沒有提早結束的工作量上限**：100 萬個陳述式要約 1.7 秒、1.6 GB 才回報程式太大。可以用 context 取消。
- **編譯器本身仍是遞迴**：`internal/vm` 的運算式編譯受 100,000 層上限保護，約 43 MB 堆疊，尚未改成迭代。
- **`exit(` 跨行時引數 Deprecated 的行號**：PHP 用右括號那一行，HIR 沒有這個資訊。
- **呼叫宿主註冊的函式**：`Config.HostFunctions` 的函式還不能從 PHP 呼叫，`Compile` 回報不支援。
- **在字串中間改寫位移仍複製整個字串**：改寫迴圈是平方時間。要原地改寫得知道字串只有一個持有者，目前字串是 Go string，沒有持有者計數。延長字串的寫入已是線性。
- **字串位移超過 PHP 的記憶體上限**：PHP 以 `Allowed memory size exhausted` 的 fatal 結束，而且會先印出同一行之後的輸出；Phego 在 128 MiB 回報 `ResourceError`。
- **參數預設值的 intern 判斷是保守的**：可能拒絕少數 PHP 能正常執行的預設值。
- **字串串接每次都複製整個字串**：迴圈裡用 `.` 累加字串是平方時間。PHP 在沒有其他持有者時會原地附加。
- **編譯期警告以 E_WARNING 過濾**：PHP 用的是 E_COMPILE_WARNING，error_reporting 只關其中一個位元時結果會不同。
- **編譯期 fatal 之前的編譯期警告會遺失**：`Compile` 只回傳 `PHPError`。
- **函式隱含的 return 行號**：使用函式內出現過的最大行號，PHP 使用右大括號那一行。階段一觀察不到差別。
- **兩個元素之間參照指定的運算式值**：`$x = ($a[0] = &$b[0])` 在 `Compile` 回報不支援。單獨成為陳述式的 `$a[0] = &$b[0];` 可以執行。
- **傳參照的引數只檢查到編譯時已知的函式**：直接呼叫時以 MakeRef 傳入；bytecode 的函式表沒有記錄哪些參數傳參照，所以驗證器無法檢查手寫的 bytecode 有沒有對傳值參數傳入參照。被呼叫端的 Store 會解開參照，不會造成記憶體問題。
- **`error_reporting` 的 INI 值只接受十進位整數**：PHP 的 `-d` 也接受 `E_ALL` 這類運算式。
