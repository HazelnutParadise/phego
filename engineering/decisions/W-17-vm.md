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
| 四 | 解構：`[$a, $b] = $x`、`list()`、跳過的位置、任意運算式的 key、巢狀樣式、元素目標（含 `$a[]`）、解構運算式的值、`foreach` 搭配樣式（含 key）、傳參照的元素 `[&$a]`（含巢狀與 foreach）、右邊不是陣列、缺少的 key、左右互相影響的寫法 | 物件與靜態屬性目標，`[...$rest]`（PHP 的編譯錯誤），`[&$a] = f()`，從元素解構參照的運算式值，走訪物件，其他第三階段右欄的項目 |
| 五 | 頂層與 namespace 裡的 `const`（含相依的值、陣列值、重複宣告的警告、未定義常數的錯誤、namespace 裡退回全域的查詢）、`php -n` 的 1,792 個預先定義常數（三個 profile 各自的值、已棄用常數的通知）、以常數為參數預設值、magic constant、`match`（多條件、`default`、`match(true)`、沒有命中的 `UnhandledMatchError`）、`empty()`（變數、任意層元素、字串位移、運算式）、`@`（巢狀、函式呼叫內的警告、display 與 log 兩條路、不同的 `error_reporting`） | `define()`、`constant()`、`defined()`（內建函式），STDIN／STDOUT／STDERR（資源），CHAR_MAX 與 PHP_BUILD_DATE（依 CPU 架構而異），類別常數，trait 裡的 `__CLASS__`，`global`、`static`，不定參數，具名引數，物件，try／catch |
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

每一項都把候選做法放進同一個測試執行檔，跑同樣的五個工作量，先確認輸出、回傳值與步數完全相同，再比時間。工作量是算術迴圈、分支加 `%` 與比較、以 `.` 串接字串、遞迴的 `fib(18)`、陣列填入與讀取。量測在 GitHub 的 ubuntu-24.04（amd64）與 ubuntu-24.04-arm runner 上各交替執行 10 輪，以 benchstat 比較（`tools/benchstat-variants.sh`，「VM layout benchmarks」工作流程，commit 5545d4f）。原始輸出在 [W-17 證據目錄](../implementation/evidence/W-17-vm/benchstat)。

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

## 階段四

格式升到 1.4，沒有新增指令：ArrayGet 多一個 Aux 2（ZEND_FETCH_LIST_R），FetchDim 多一個 FetchContainerValue 旗標（ZEND_FETCH_LIST_W 對 VAR 容器的寫入取元素）。1.0 到 1.3 的檔案照樣能載入，其中出現 1.4 的內容時以 `ErrInvalid` 拒絕並指出需要 1.4。操作數規則在 [W-16 文件](W-16-bytecode.md)。因為只是現有指令的新 Aux，沒有做布局量測。

- **順序**照 zend_compile_assign 的 ZEND_AST_ARRAY 分支與 zend_compile_list_assign。右邊是變數時先複製（QM_ASSIGN），所以 `[$a, $b] = $a` 與 `[$a[0], $a[1]] = $a` 讀的是原本的陣列；其他右邊只求值一次。每個元素依序：有 key 就先算 key（key 是變數時由讀取指令自己讀），用 FETCH_LIST_R 取出元素，再指定給目標；元素目標的 key 在取出元素之後才算；巢狀樣式從取出的元素再解構。PHP 8.5.12 的原始碼裡沒有 `zend_list_has_assign_to_self`，變數右邊一律複製，Phego 照做。
- **右邊不是陣列**：null 安靜讀到 null；int、float、bool、string 發出 `Cannot use T as array`（T 是型別名稱，true 與 false 都是 bool）並讀到 null；字串不當成字串位移。缺少的 key 發出 `Undefined array key`。
- **傳參照的元素**：右邊先變成參照（MakeRef）放進暫存器，整個指定期間持有；要綁參照的元素（以及含參照的巢狀樣式）用 FetchDim（FetchWrite、FetchForRef、FetchContainerValue）從這個參照取元素，再 MakeRef、Bind；其他元素透過參照用 FETCH_LIST_R 讀。指定結束時以 Drop 釋放暫存器裡的參照，不等 liveness：PHP 在陳述式結束時釋放 VAR，之後複製陣列時參照計數為 1 的元素會被拆開，`internal/runtime/vm_list_test.go` 用跳過 liveness 的函式證明少了 Drop 時複製會共用元素。巢狀含參照的子樣式的 VAR 也一樣在子樣式結束時 Drop。`foreach` 的樣式裡有參照時整個迴圈變成傳參照（zend_propagate_list_refs），IterNext 把元素的參照放進暫存器，樣式綁完後 Drop。
- **行號**照 zend_compile_list_assign 留下的 `CG(zend_lineno)`：第一個元素的 FETCH 在右邊結束的行；變數目標指定後，行移到那個變數的行；元素目標與參照目標留在 FETCH 的行；右邊是變數時，複製發生在樣式第一個元素的行。為了讓多行的寫法逐位元組相同，另外修正兩個前端與 HIR 的行號：空的陣列字面值取右括號那一行（zend_ast_create_list 沒有子節點時取當時的行），兩個字面值的 `.` 由 PHP 的 parser 折疊成一個字串（zend_ast_create_concat_op），行號是右運算元後面那個 token 的行，HIR 以 `FoldLine` 記下，VM 在 Concat 之後把行停在那裡。
- **編譯期拒絕**：`[...$rest]`、空樣式、混用 key、混用 `[]` 與 `list()`、`array()` 當目標、對不能參照的值取參照、`foreach` 的 key 是樣式，都照 PHP 的編譯錯誤訊息，由 lowering 回報。物件與靜態屬性目標回報帶位置的 `UnsupportedError`。

獨立審查跑了 118 個 PHP 探測程式，另以 1,300 層巢狀的超大函式確認關閉 liveness 時暫存器沒有殘留可見的參照。沒有 P0 或 P1；找到的問題都已修正並加上修正前會失敗的回歸 fixture（修正前的輸出在 [stage4/red-review-fixes.txt](../implementation/evidence/W-17-vm/stage4/red-review-fixes.txt)）：

- **透過參照碰到 false 容器時多印 Deprecated**（P2）。zend_fetch_dimension_address 的 IS_REFERENCE 分支對 null 與 false 直接 array_init，unset 模式直接回 null，都不呼叫 zend_false_to_array_deprecated；只有 ASSIGN_DIM 與 ASSIGN_DIM_OP 會先解參照再警告。傳參照解構的容器一定是參照，所以 `[&$a] = $f`（`$f = false`）每次都多一行；`$r = &$h; $h[0][1] = 1`、`$x = &$m[0]`、`unset($m[0][1])` 從第二階段起就有同樣的問題。現在 FetchDim 的容器是參照時不發這個通知，ASSIGN_DIM_OP 的最後一層除外。
- **樣式裡的編譯錯誤行號**（P3）。lowering 在每個元素之後把目前行停在目標的行，PHP 則是元素目標與參照目標都留在取值那一行、變數目標移到變數那一行；foreach 的樣式從主體結束的行開始。七個 `fatal_list_lines_*` fixture 的 `Cannot mix`、`Spread operator` 與 `Cannot use empty array entries` 行號現在和 PHP 相同。
- **從元素解構參照的運算式值與 `[&$a] = f()`**（P3）在 `Compile` 回報帶位置的不支援，不會跑出錯的輸出；列在已知缺口。審查者建議的做法是 Drop 之前先從參照讀值，但目前沒有把 Value 暫存器裡的參照解開複製的指令，留給之後的階段。

測試程式加到 263 個（新增 24 個 `list_*`、`foreach_list_*`、`ref_*` 與 18 個 `fatal_list_*`），三個 profile 各 789 項全部和 PHP 8.5.12 逐位元組相同。

## 階段五

格式升到 1.5，新增 FetchConst、DeclareConst、BeginSilence、EndSilence 與 MatchError 五個指令，以及 ArrayIsset 的 Aux 1（`empty($a[k])`）。1.0 到 1.4 的檔案照樣能載入，其中出現 1.5 的內容時以 `ErrInvalid` 拒絕並指出需要 1.5。操作數規則在 [W-16 文件](W-16-bytecode.md)。這五個指令都沒有現有指令能組合出來：沒有指令能讀寫 error_reporting、查詢或登錄常數，也沒有指令能拋出帶格式訊息的錯誤；`match` 的比較與分支、`empty()` 的變數與運算式形式則用 Identical、Branch、安靜的 Load 與 Not 組合。分派用的 `switch` 多了五個 case，以 main 與本分支各自建置的測試執行檔交替各跑 6 輪 `BenchmarkVMDispatch/*/switch`，五個工作量的中位數差在 −3.4% 到 +5.4% 之間，每格的 6 輪範圍互相重疊，原始輸出在 [stage5/bench-main.txt](../implementation/evidence/W-17-vm/stage5/bench-main.txt) 與 [stage5/bench-stage5.txt](../implementation/evidence/W-17-vm/stage5/bench-stage5.txt)。

- **預先定義的常數**在 `internal/phpconst`，由 `gen/constants.php` 在固定映像裡用 `get_defined_constants()` 產生，64 位元的表來自 arm64 映像、32 位元的表來自 386 映像，每個名稱記下值、型別，以及 `ReflectionConstant::isDeprecated()` 為真時實際觸發的 Deprecated 訊息（E_STRICT 等 56 個）。amd64 映像的表和 arm64 的只有 CHAR_MAX（127 對 255）與 PHP_BUILD_DATE 不同，兩者記為「依平台而異」：profile 目前沒有 CPU 架構欄位，程式讀到它們時 `Compile` 回報帶位置的不支援，不猜值。STDIN、STDOUT、STDERR 是資源，同樣只記名稱並拒絕。表唯讀且可共用。
- **編譯期折疊**照 zend_try_ct_eval_const：不需要執行期退回全域名稱、而且不是已棄用的預先定義常數，在編譯時換成它的值；其他名稱（使用者常數、namespace 裡未限定的名稱、已棄用的常數）用 FetchConst 在執行時查。查詢的鍵是 namespace 部分小寫、常數名稱照寫法，和 zend_register_constant 一致，所以 `\A\c` 找得到 `namespace a\B; const c`，而 `\B\C` 找不到。未定義常數的錯誤文字用原始寫法（含 namespace 前綴）。
- **使用者常數**屬於當次執行：DeclareConst 把值登錄進 vmState 的表，執行結束時釋放，Program 不會留下任何一次執行的常數，同一個 Program 的多次執行與並行執行互不相見（`internal/runtime/vm_stage5_test.go`）。重複宣告、宣告預先定義的名稱、`__COMPILER_HALT_OFFSET__` 與大小寫不分的 true／false／null 都只發出 PHP 的 `Constant %s already defined` 警告。
- **宣告的行號**照 zend_compile_const_decl：值在 DECLARE_CONST 執行時才求值（zval_update_constant_ex），所有診斷都帶那條指令的行號，而那個行號是 CG(zend_lineno) 當時的值：陳述式的行（第一個常數名稱的行），被值裡每個編譯時折疊不了的常數引用依序移到它的行（zend_compile_const_expr_const），而且同一個陳述式裡不會移回來。所以 `const A = 1,\n B =\n UNDEF\n + 1;` 的錯誤在 UNDEF 那一行，`const A = 1,\n B = C,\n D = 2 + [];` 的 TypeError 在 C 那一行。值的程式碼以 `pinned` 固定在這個行號編譯。
- **match** 照 zend_compile_match 的兩種形式。至少兩個條件而且全部是編譯時已知的整數或字串時，PHP 用 jump table（ZEND_MATCH），變數主體只讀一次、在主體的行發出未定義警告；否則每個條件依序編譯，再以 CASE_STRICT／IS_IDENTICAL 比較，變數主體在每次比較時重讀，警告在條件的行。兩種形式都用 Identical 與 Branch 表達，差別只在主體變數何時 Load。沒有 `default` 時走到 MatchError，它讀未定義的主體變數不發警告，行號在 jump table 形式是主體的行、否則是最後一個條件的行。訊息照 smart_str_append_scalar：字串加引號、截到 15 個 byte 加 `...`，浮點帶 `.0`，陣列是 `of type array`。
- **match 條件的折疊**照 PHP 編譯器的順序：can_match_use_jumptable 依序對每個條件做 zend_eval_const_expr，遇到第一個不是整數或字串的條件就停止。折疊出新 zval 的條件（true／false／null、magic constant、預先定義常數、常數上的運算、cast、陣列字面值）會拿到當時 CG(zend_lineno) 的值，也就是主體的行；掃描器產生的純量與 parser 折疊的字串串接保留自己的行；條件為常數的 `?:` 與 `??` 換成選到的那個運算元，保留它自己的行；停止之後的條件保留自己的行。這由 lowering 在 `internal/ir` 的 `match()` 做同樣的折疊（`evalMatchCondition`），編譯器只看到折疊後的字面值，jump table 的判斷也以它們為準。為了讓 `PHP_INT_SIZE * 2` 這類運算式能折疊，IR 的常數求值把不需要退回全域、也沒有被棄用的預先定義常數當成編譯期的值，和 zend_try_ct_eval_const 一致。
- **empty()** 照 zend_compile_isset_or_empty：變數是安靜的 Load 接 Not（ISSET_ISEMPTY_CV），元素是每一層安靜的 ArrayGet 接 ArrayIsset Aux 1（ISSET_ISEMPTY_DIM_OBJ 的 ZEND_ISEMPTY），其他運算式是 `!expr`。字串容器照 zend_isempty_dim_slow 以位置判斷：位置在範圍內時只有 `"0"` 算空；`"1x"` 這類開頭是數字的字串位移在 `empty()` 不發警告，在讀取時發 `Illegal string offset`，所以元素形式不能用安靜讀取加 Not 組合。
- **@** 照 zend_compile_silence：BeginSilence 把 error_reporting 存進暫存器並只留下 E_FATAL_ERRORS 的位元，運算式在中間編譯（變數也在裡面才 Load，對應 PHP 強制的 FETCH），EndSilence 在目前值只剩 fatal 位元時還原存下的值；巢狀的內層存到的已是只剩 fatal 的值，所以由外層還原。error_reporting 是 vmState 的欄位，`errorChannels.excluded` 跟著它，每次執行從 INI 值開始，一個請求的 `@` 不影響另一個請求。fatal（未捕捉的錯誤）照樣顯示，因為 E_ERROR 留在遮罩裡；`exit` 與未捕捉錯誤結束執行時不需要還原，PHP 在同樣情況下也沒有可觀察的差別。
- **FetchConst 的執行期快取**照 ZEND_FETCH_CONSTANT 的 cache slot：每條指令在一次執行裡第一次解析到的常數（已棄用的除外）會記住，之後都用同一個值。所以 namespace 裡退回全域的名稱，在之後宣告了同名的 namespace 常數後，同一條指令仍讀到全域的值，和 PHP 相同；重複讀取也省掉查表。快取屬於當次執行。
- **對照測試的 INI 覆寫**：fixture 在開頭標籤後的 `// ini: error_reporting=30717` 這類註解會由 `reference.php` 加成 `-d`，也由 `w17_differential_test.go` 加成 `WithINI`，用來測 error_reporting 不同時 `@` 與一般診斷的結果（只顯示 fatal、全部關閉、關掉 warning 但留 deprecated）。

獨立審查跑了 178 個 PHP 探測程式（amd64 映像全部，386 與 arm64 各 163 與 134 個）、96 個並行執行的隔離與 race 檢查、15 個手寫 bytecode 程式，沒有 P0 或 P1；找到的問題都已修正並加上修正前會失敗的回歸 fixture（修正前的輸出在 [stage5/red-review-fixes.txt](../implementation/evidence/W-17-vm/stage5/red-review-fixes.txt)）：

- **match 條件的折疊不完整**（P2）。原本只有單獨的常數名稱與 IR 已折疊的字面值被當成 jump table 的鍵，`"a" . "b"`、`PHP_INT_SIZE * 2`、`(int) "5"`、`~1`、`true ? 1 : 2` 這類條件走逐條比較，未定義的主體變數就多印警告；沒有 `default` 時 `UnhandledMatchError` 的行號也不同。現在 lowering 照 zend_eval_const_expr 折疊這些條件（`match_jumptable_folded_conditions`、`fatal_match_lines_folded_expression`）。
- **namespace 裡未限定的資源與平台常數在執行期才拒絕**（P2）。`namespace A; echo STDIN;` 原本能通過 `Compile`，執行到那一行才以 Unsupported 結束，輸出已經寫出一部分。單一單元沒有 include、eval 與 `define()`，所以單元沒宣告 `A\STDIN` 時退回全域是確定的，現在 `Compile` 就帶位置拒絕（`internal/vm/compile_test.go`）。
- **退回全域的名稱每次重新解析**（P3）。PHP 把第一次解析的結果記在指令的 cache slot，之後 namespace 裡再宣告同名常數也不會改變那條指令讀到的值；Phego 原本每次重查，所以第二次呼叫讀到新宣告的值。現在每條 FetchConst 在一次執行裡記住第一次的結果（`const_namespace_fallback_cache`）。
- 決策紀錄原本寫「只有 match 條件的行號依賴這個折疊」，但 jump table 的判斷（也就是警告的次數）同樣依賴它；已改寫。

測試程式加到 326 個（新增 63 個 `const_*`、`magic_*`、`match_*`、`empty_*`、`silence_*`、`return_const_profile` 與對應的 `fatal_*`），三個 profile 各 978 項全部和 PHP 8.5.12 逐位元組相同。

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
- **從元素解構參照的運算式值**：`$y = ([&$a] = $x[0])` 在 `Compile` 回報不支援；`$y = ([&$a] = $x)` 與單獨成為陳述式的寫法可以執行。
- **`[&$a] = f()`**：PHP 會發出 `Attempting to set reference to non referenceable value` 的 notice 再照值讀取；Phego 沿用第三階段「參照到函式呼叫結果」的不支援回報。執行期 FetchDim 的容器暫存器不是參照時也以 Unsupported 結束，編譯器不會產生這種 bytecode。
- **傳參照的引數只檢查到編譯時已知的函式**：直接呼叫時以 MakeRef 傳入；bytecode 的函式表沒有記錄哪些參數傳參照，所以驗證器無法檢查手寫的 bytecode 有沒有對傳值參數傳入參照。被呼叫端的 Store 會解開參照，不會造成記憶體問題。
- **`error_reporting` 的 INI 值只接受十進位整數**：PHP 的 `-d` 也接受 `E_ALL` 這類運算式。
- **依 CPU 架構而異的預先定義常數**：CHAR_MAX（arm64 為 255，amd64 與 386 為 127）與 PHP_BUILD_DATE 在 `Compile` 回報不支援；profile 要有架構欄位才能給值。
- **STDIN、STDOUT、STDERR**：值是資源，`Compile` 回報不支援，等資源與串流的工作包。
- **預先定義常數上的運算只在常數運算式與 match 條件裡折疊**：PHP 把一般運算式裡的 `PHP_INT_MAX + 1` 也折成常數，Phego 在執行時算，結果相同，目前沒有觀察到可見的差別。`(int) "5x"` 這類 PHP 能無聲轉換、Phego 的常數求值拒絕的 cast 在 match 條件裡不折疊，主體是未定義變數時警告的次數可能不同。
- **match 沒有 jump table**：PHP 對至少兩個整數或字串字面值的條件用 hash 查表，Phego 依序比較，條件很多時較慢，結果相同。
- **`define()`、`constant()`、`defined()`**：內建函式，屬於模組工作包；`const` 以外沒有辦法在執行期宣告常數。
