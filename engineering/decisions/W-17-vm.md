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
| 六 | `global`（函式內與頂層、不存在時建立 null、`unset` 只解除函式裡的綁定、宿主以 `WithGlobal` 提供的值）、`static` 變數（字面值與任意運算式的初始值、第一次執行到才求值、多個宣告、遞迴）、不定參數（`...$rest`、`&...$refs`、接在有預設值的參數後面）、引數展開（`f(...$a)`、多段展開、字串 key 成為具名引數、對傳參照參數建立參照）、具名引數（任意順序、略過有預設值的參數、多出的名稱收進不定參數、傳參照） | `$GLOBALS`、`global $$name`，`func_get_args()` 等內建函式，展開物件與 Traversable，把不是變數的值以名稱傳給傳參照參數，參數型別，類別與方法，closure，try／catch |
| 七 A | 類別、介面、trait 的宣告（含提早綁定與條件式宣告、繼承規則的 fatal）、`new`、屬性與 static 屬性的讀寫（typed、readonly、動態屬性、陣列寫入、複合指定、`++`／`--`、`??=`、`unset`、`isset`、`empty`、參照）、方法與 static 方法、`$this`、`self`／`parent`／`static`、`::class`、類別常數、參數與回傳型別、magic method、`instanceof`、`clone`、物件比較與轉型、走訪物件、nullsafe、`$callable()` | closure 與箭頭函式、first-class callable（七 B），try／catch／finally、`throw`、例外類別（七 C），enum、property hooks、非對稱可見度、屬性與類別上的 attribute、匿名類別、回傳參照、generator、stdClass 與 Stringable 以外的內建類別、把元素或屬性傳給執行期才知道的被呼叫者的傳參照參數 |
| 七 B | closure 物件（呼叫、具名與展開引數、傳參照參數、型別、`instanceof Closure`、比較、`clone`、轉型）、`use` 傳值與傳參照捕捉、closure 裡的 `static` 變數、方法裡自動綁定的 `$this` 與類別範圍、static closure、箭頭函式與自動捕捉、first-class callable（`f(...)`、`$o->m(...)`、`A::m(...)`、`static::m(...)`、`$closure(...)`、`'f'(...)`、陣列 `[$o, 'm']` 接 `(...)`）、以字串 `A::m` 與陣列呼叫方法、`Closure::bind`／`bindTo`／`call`／`fromCallable`、把元素或屬性傳給執行期才知道的被呼叫者的傳參照參數 | try／catch／finally、`throw`、例外類別（七 C），generator，回傳參照的函式與 closure（`fn &` 除外），內建函式的 `strlen(...)`、需要 callable 的內建函式、Reflection，其他七 A 右欄的項目 |
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
- **match 條件的折疊**照 PHP 編譯器的順序：can_match_use_jumptable 依序對每個條件做 zend_eval_const_expr，遇到第一個不是整數或字串的條件就停止。折疊出新 zval 的條件（true／false／null、magic constant、預先定義常數、常數上的運算、cast、常數陣列或字串的元素、陣列字面值）會拿到當時 CG(zend_lineno) 的值，也就是主體的行；掃描器產生的純量（HIR 的 `FlagScanned`）與 parser 折疊的字串串接保留自己的行；條件為常數的 `?:` 與 `??` 換成選到的那個運算元，保留它自己的行；整個條件折不了時，裡面能折的運算元照樣換成主體行上的 zval（`$a + PHP_INT_SIZE` 的比較因此落在主體的行）；停止之後的條件保留自己的行。這由 lowering 在 `internal/ir` 的 `match()` 做同樣的折疊（`evalMatchCondition`），編譯器只看到折疊後的字面值，jump table 的判斷也以它們為準。為了讓 `PHP_INT_SIZE * 2` 這類運算式能折疊，IR 的常數求值把不需要退回全域、也沒有被棄用的預先定義常數當成編譯期的值，和 zend_try_ct_eval_const 一致；cast 照 zend_try_ct_eval_cast（字串以數字開頭的部分轉換、NAN 與浮點轉字串不折）。
- **empty()** 照 zend_compile_isset_or_empty：變數是安靜的 Load 接 Not（ISSET_ISEMPTY_CV），元素是每一層安靜的 ArrayGet 接 ArrayIsset Aux 1（ISSET_ISEMPTY_DIM_OBJ 的 ZEND_ISEMPTY），其他運算式是 `!expr`。字串容器照 zend_isempty_dim_slow 以位置判斷：位置在範圍內時只有 `"0"` 算空；`"1x"` 這類開頭是數字的字串位移在 `empty()` 不發警告，在讀取時發 `Illegal string offset`，所以元素形式不能用安靜讀取加 Not 組合。
- **@** 照 zend_compile_silence：BeginSilence 把 error_reporting 存進暫存器並只留下 E_FATAL_ERRORS 的位元，運算式在中間編譯（變數也在裡面才 Load，對應 PHP 強制的 FETCH），EndSilence 在目前值只剩 fatal 位元時還原存下的值；巢狀的內層存到的已是只剩 fatal 的值，所以由外層還原。error_reporting 是 vmState 的欄位，`errorChannels.excluded` 跟著它，每次執行從 INI 值開始，一個請求的 `@` 不影響另一個請求。fatal（未捕捉的錯誤）照樣顯示，因為 E_ERROR 留在遮罩裡；`exit` 與未捕捉錯誤結束執行時不需要還原，PHP 在同樣情況下也沒有可觀察的差別。
- **FetchConst 的執行期快取**照 ZEND_FETCH_CONSTANT 的 cache slot：每條指令在一次執行裡第一次解析到的常數（已棄用的除外）會記住，之後都用同一個值。所以 namespace 裡退回全域的名稱，在之後宣告了同名的 namespace 常數後，同一條指令仍讀到全域的值，和 PHP 相同；重複讀取也省掉查表。參數預設值的程式碼不用這個快取（Aux 的 FetchConstNoCache）：PHP 用 zval_update_constant_ex 求預設值，沒有 cache slot，所以 `$d = PHP_OS . "x"` 每次呼叫都重新查，之後宣告的 namespace 常數會被看到；預設值就是一個常數名稱時（FetchConstDefault），純量結果記成參數的預設值快取，和 RECV_INIT 保留不計數的結果一樣，陣列則每次重新求。快取屬於當次執行，正常結束時釋放；fatal 或宿主停止時和其他執行狀態一樣留給請求拆除。
- **對照測試的 INI 覆寫**：fixture 在開頭標籤後的 `// ini: error_reporting=30717` 這類註解會由 `reference.php` 加成 `-d`，也由 `w17_differential_test.go` 加成 `WithINI`，用來測 error_reporting 不同時 `@` 與一般診斷的結果（只顯示 fatal、全部關閉、關掉 warning 但留 deprecated）。

獨立審查跑了 178 個 PHP 探測程式（amd64 映像全部，386 與 arm64 各 163 與 134 個）、96 個並行執行的隔離與 race 檢查、15 個手寫 bytecode 程式，沒有 P0 或 P1；找到的問題都已修正並加上修正前會失敗的回歸 fixture（修正前的輸出在 [stage5/red-review-fixes.txt](../implementation/evidence/W-17-vm/stage5/red-review-fixes.txt)）：

- **match 條件的折疊不完整**（P2）。原本只有單獨的常數名稱與 IR 已折疊的字面值被當成 jump table 的鍵，`"a" . "b"`、`PHP_INT_SIZE * 2`、`(int) "5"`、`~1`、`true ? 1 : 2` 這類條件走逐條比較，未定義的主體變數就多印警告；沒有 `default` 時 `UnhandledMatchError` 的行號也不同。現在 lowering 照 zend_eval_const_expr 折疊這些條件（`match_jumptable_folded_conditions`、`fatal_match_lines_folded_expression`）。
- **namespace 裡未限定的資源與平台常數在執行期才拒絕**（P2）。`namespace A; echo STDIN;` 原本能通過 `Compile`，執行到那一行才以 Unsupported 結束，輸出已經寫出一部分。單一單元沒有 include、eval 與 `define()`，所以單元沒宣告 `A\STDIN` 時退回全域是確定的，現在 `Compile` 就帶位置拒絕（`internal/vm/compile_test.go`）。
- **退回全域的名稱每次重新解析**（P3）。PHP 把第一次解析的結果記在指令的 cache slot，之後 namespace 裡再宣告同名常數也不會改變那條指令讀到的值；Phego 原本每次重查，所以第二次呼叫讀到新宣告的值。現在每條 FetchConst 在一次執行裡記住第一次的結果（`const_namespace_fallback_cache`）。
- 決策紀錄原本寫「只有 match 條件的行號依賴這個折疊」，但 jump table 的判斷（也就是警告的次數）同樣依賴它；已改寫。

Codex 的審查額度在 PR 開出時已用完，依本階段的規則改由第二位獨立審查者檢查上述修正（efe13b4..ba401b4）：64 個探測程式在 amd64 與 386 映像對照、32 個 goroutine 各 20 次的隔離與 race 檢查，沒有 P0 或 P1。找到並修正：

- **FetchConst 快取套用到參數預設值**（P2，回歸）。`namespace A; function g($d = PHP_OS . "x")` 在 PHP 每次呼叫重新求值，之後宣告的 `A\PHP_OS` 會被看到；快取讓 Phego 一直給舊值。預設值程式碼改為不用指令快取，常數名稱本身當預設值時改記進參數預設值快取（`const_default_fallback_cache`）。
- **短路邏輯折出的字面值行號**（P2，回歸）。lowering 把 `true || $a` 折成字面值時沒有標記，被當成掃描器的純量而保留自己的行。改成反過來標記掃描器產生的字面值（`FlagScanned`），其他字面值一律視為折疊出來的。
- **整個條件折不了時運算元沒有折**、**常數陣列或字串的元素**、**以數字開頭的字串 cast 與 `(array)`**（P2）：`$a + PHP_INT_SIZE`、`[10, 20][1]`、`(int) "12abc"` 現在都照 zend_eval_const_expr 處理（`match_folded_operands_and_elements`）。
- **`#[\Deprecated] const`**（P2）：屬性原本被靜默忽略，現在在 `Compile` 回報不支援。
- **`E_ALL ^ E_WARNING` 當參數預設值**（P3）原本被 intern 判斷拒絕；預先定義常數的非字串值現在算進已知不是字串的運算元（`const_expr_predefined`）。

測試程式加到 329 個（新增 63 個 `const_*`、`magic_*`、`match_*`、`empty_*`、`silence_*`、`return_const_profile` 與對應的 `fatal_*`），三個 profile 各 987 項全部和 PHP 8.5.12 逐位元組相同。

## 階段六

格式升到 1.6，新增 BindGlobal、StaticInit、BindStatic、RecvVariadic、InitCall、SendUnpack 與 CallPending 七個指令、Push 的 Aux PushLoad，以及每個函式的參數表（名稱與是否傳參照、是否有不定參數）。1.0 到 1.5 的檔案照樣能載入，其中出現 1.6 的內容時以 `ErrInvalid` 拒絕並指出需要 1.6。操作數規則在 [W-16 文件](W-16-bytecode.md)。這七個指令都沒有現有指令能組合出來：沒有指令能碰到別的 frame 的變數或跨呼叫保存的值，也沒有指令能讀到超過參數個數的引數，或在執行期才決定引數的個數與位置。位置引數的呼叫仍走原本的 Push 與 Call，不經過新指令。分派用的 `switch` 多了七個 case，Return 多一個「暫存器裡是不是參照」的判斷。以 main（56f8bf5）與本分支各自建置的測試執行檔交替各跑 6 輪 `BenchmarkVMDispatch/*/switch`，五個工作量的中位數差在 −6.2% 到 +5.6% 之間，每格的 6 輪範圍互相重疊，各格最快的一輪相差 +0.3% 到 +1.9%。量測時主機上有其他工作在跑，數字比第五階段雜，原始輸出在 [stage6/bench-main.txt](../implementation/evidence/W-17-vm/stage6/bench-main.txt) 與 [stage6/bench-stage6.txt](../implementation/evidence/W-17-vm/stage6/bench-stage6.txt)。

- **global** 照 ZEND_BIND_GLOBAL：函式裡的變數變成頂層同名變數的參照，頂層沒有這個變數時建立 null。頂層變數就是 `{main}` 的 local，`Program` 只記名稱到 local 的對照表。`{main}` 從沒提過的名稱放在當次執行自己的表裡，宿主以 `WithGlobal` 提供的同名值在第一次用到時匯入，所以只在函式裡出現的全域變數也看得到宿主的值。`unset($a)` 與 `$a = &$other` 只改函式裡的綁定。在頂層寫 `global $x` 只會讓未定義的 `$x` 變成 null。
- **static 變數** 的值放在當次執行的表裡，以函式和變數為鍵，執行結束時釋放，`Program` 不留任何一次執行的值。同一個 `Program` 的多次執行各自從初始值開始，並行執行互不相見（`internal/runtime/vm_stage6_test.go` 與 `w17_stage6_test.go` 各以 16 個 goroutine 同時執行同一個 `Program`）。PHP 對字面值初始值只用 ZEND_BIND_STATIC，對其他運算式用 ZEND_BIND_INIT_STATIC_OR_JMP 加 ZEND_BIND_STATIC。編譯器兩種都產生後者的形式，因為字面值在第一次執行時求值會得到同樣的值。初始值拋出錯誤時變數保持未初始化，下一次呼叫重新求值。初始值裡又呼叫同一個函式時，內層先完成初始化，外層算出的值被丟棄（`static_recursion` 的 `init_reenters`）。
- **不定參數** 不占參數暫存器：bytecode 的 Params 只算前面的固定參數，多出的引數照舊留在 frame 裡（backtrace 本來就要顯示它們），RecvVariadic 把它們和多出的具名引數收成陣列。少傳引數時的訊息照 zend_missing_arg_error，以固定參數判斷 `exactly` 或 `at least`，所以 `function f($a, $b, ...$rest)` 少傳時是 `exactly 2 expected`。
- **具名引數與展開** 的呼叫在引數之間就要檢查，所以這種呼叫的每個引數求值後立刻送出（InitCall、逐個 Push 或 SendUnpack、CallPending），不像位置引數那樣全部求值完才 Push。PHP 的錯誤時機因此相同：`f(a: g(), nope: h(), b: k())` 在 `h()` 執行後、`k()` 執行前拋出 `Unknown named parameter $nope`。引數是單純變數時由 Push 自己讀（PushLoad），名稱的檢查在讀取之前，所以名稱錯誤時不會先印未定義變數的警告。引數是元素時先讀取才檢查，和 PHP 的 FETCH 在 SEND 之前相同（`fatal_named_unknown_variable_arg` 與 `fatal_named_unknown_element_arg`）。
- **略過的參數** 照 zend_handle_undef_args：具名引數跳過的參數在引數堆疊上留下空位，進入函式後由 ParamDefault 當成沒傳來求值，預設值的警告帶參數那一行，並照樣進入預設值快取。空位的參數沒有預設值時，在被呼叫函式的 frame 裡拋出 `ArgumentCountError: f(): Argument #N ($name) not passed`，backtrace 的那一格顯示 NULL。PHP 在 ZEND_CHECK_UNDEF_ARGS 處理空位，這條指令帶最後一個引數的行號，呼叫本身帶呼叫那一行。為了讓跨行的呼叫逐位元組相同，編譯器在 CallPending 前放一條帶最後引數行號的 Checkpoint，被呼叫函式還在填空位時，backtrace 用它的行號。
- **展開與參照** 照 ZEND_SEND_UNPACK：展開的是變數，而且有元素要傳給傳參照的參數時，元素在變數自己的陣列裡變成參照，陣列被其他變數共用時先分離。展開的是函式回傳值、字面值或元素讀取的結果時，參照建立在暫存的陣列上，原本的陣列不變（`unpack_by_ref`）。字串 key 走具名引數的路徑，之後再出現整數 key 是 `Cannot use positional argument after named argument during unpacking`。
- **PHP 編譯呼叫時認不認得函式**會影響行號。函式宣告在呼叫之後、函式呼叫自己、namespace 裡未限定的名稱，或引數排在展開之後時，PHP 還不知道被呼叫的是誰（zend_compile_call 在函式表裡找不到已完成的函式），這時單純變數的引數會帶引數列的行號，也就是第一個引數最左邊那個運算元的行，未定義變數的警告就報在那一行。編譯器以宣告順序重現這個判斷（`calleeKnown`），位置引數的呼叫也一併照做，這在前幾個階段是和 PHP 不同的（`call_callee_declared_later`）。
- **暫存器不留值**：BindStatic 與 RecvVariadic 後面的 Bind 都把值移出暫存器，具名的 Push 把暫存器裡的參照移交給呼叫，所以不管 liveness 有沒有執行，static 變數與不定參數的陣列都只有一個持有者（`TestStage6InstructionsEmptyTheirRegisters` 用跳過 liveness 的函式檢查）。函式回傳時 Return 把暫存器裡的參照換成它的值。
- **補上第三階段的缺口**：函式表記錄每個參數是否傳參照。格式 1.6 的驗證器追蹤哪些 Value 暫存器可能存放參照，拒絕以 Call 把參照送給記錄為傳值的參數。以名稱呼叫（CallValue）與 CallPending 在執行期把送給傳值參數的參照換成值。

獨立審查先後由兩位審查者進行，都只拿到需求與程式碼。第一位（agy 的 Opus）跑了 148 個 PHP 探測後額度用完，沒有交出報告。重跑它留下的探測，147 個和 PHP 相同，剩下 1 個用到本來就不支援的 `[&$x]`。Codex 在這台機器上沒有符合條件的模型。第二位（內建的 Opus，全新的脈絡）另外跑了 183 個探測（arm64 映像，另有 20 個在 386 映像）、13 個手寫 bytecode 程式與逐套件的 race 檢查，沒有 P0。找到的問題都已修正並加上修正前會失敗的回歸測試，修正前的輸出在 [stage6/red-review-fixes.txt](../implementation/evidence/W-17-vm/stage6/red-review-fixes.txt)：

- **CallValue 能取走具名引數**（P1，只有手寫 bytecode 能觸發）。驗證器把帶名稱的 Push 算成引數堆疊上的一筆，CallValue 可以把它當引數取走。但被不定參數收下的名稱在執行期不會留下待處理的引數，CallValue 於是取走了下面那一筆，也就是呼叫者自己那次呼叫的引數，接著的 SendUnpack 還會以負的位置查表而讓這次執行以內部錯誤結束。現在格式 1.6 的驗證器不讓 CallValue 與 HostCall 取用帶名稱的引數，只有 CallPending 能收，SendUnpack 也先檢查引數堆疊沒有低於這次呼叫的起點（`TestNamedArgumentIsNotTakenByACallByName`）。
- **傳值的不定參數收到參照**（P2，同樣只有手寫 bytecode）。把 Location 或 MakeRef 的暫存器送給傳值不定參數時，`$rest` 陣列裡放的是呼叫者變數的參照，寫入 `$rest[0]` 會改到呼叫者的變數。RecvVariadic 現在對傳值的不定參數取參照的值（`TestByValueVariadicReceivesValues`）。
- **和呼叫同一行宣告的函式被當成 PHP 不認得**（P2，位置引數呼叫的回歸）。`function f($a, $b) {} f(1,\n$u);` 的警告應該在變數那一行。HIR 沒有欄位資訊，現在同一行的宣告一律視為在呼叫之前，這是這種寫法實際的順序（`call_callee_known_lines`）。
- **引數列的行號有兩種情況沒算對**（P3）。第一個引數是具名引數時要用名稱那一行，HIR 的引數現在記下這一行。第一個引數是 parser 折疊過的字串串接時要用折疊後的行（`FoldLine`）。heredoc 當第一個引數的情況仍然不同，列在已知缺口。
- **把不是變數的值以名稱或不定參數傳給傳參照的參數**（P3）仍是 `Compile` 的不支援回報，和位置引數在前幾個階段的做法相同，列在已知缺口。

Codex 的審查額度在 PR 開出時已用完，依本階段的規則改由另一位獨立審查者檢查上述修正（4245736..0ebd83e）。agy 仍沒有額度，Codex 沒有符合條件的模型，所以由內建的 Opus 在全新的脈絡進行：246 個探測在 arm64 映像、38 個在 386、5 個在 amd64，另有 7 個手寫 bytecode 測試，並把各項修正逐一拿掉，確認對應的回歸測試會失敗。沒有 P0、P1 或 P2，三個 P3：

- **鍵值陣列當第一個引數時的引數列行號**。Zend 的陣列元素節點把值放在鍵前面，所以 `['k'\n=>\n1]` 的行是值那一行，Phego 取了鍵那一行。這在第一輪審查前就不同，現在往下找最左邊的運算元時，陣列元素先走值（`call_callee_known_lines` 的 `later_keyed`）。
- **HostCall 不能取用具名引數的規則沒有測試**。把那幾行拿掉，測試照樣通過。已在 `internal/bytecode/format16_test.go` 補上。
- **行號由下一個 token 決定的引數**。第一個引數是 `__LINE__`、`__FILE__` 這類 magic constant，或名稱是 `array`、`list`、`fn`、`function`、`match` 的具名引數，而且後面的逗號或冒號在下一行時，PHP 用的是下一行。這在 main 上就不同，列在已知缺口。

測試程式加到 384 個（新增 55 個 `global_*`、`static_*`、`variadic_*`、`unpack_*`、`named_*`、`call_callee_*` 與對應的 `fatal_*`），三個 profile 各 1,152 項全部和 PHP 8.5.12 逐位元組相同。

## 階段七 A

第七階段分三段交付：A 把 W-13 的物件模型接上 VM，B 是 closure、箭頭函式與 first-class callable，C 是 try／catch／finally 與例外物件。這一節是 A。

格式升到 1.7，新增類別表、函式的旗標與宣告型別，以及二十個指令。1.0 到 1.6 的檔案照樣能載入，其中出現 1.7 的內容時以 `ErrInvalid` 拒絕並指出需要 1.7。類別表與操作數規則在 [W-16 文件](W-16-bytecode.md)。物件的語意全部由 W-13 的 runtime 操作決定（`internal/runtime/object_*.go`），VM 只負責指令、求值順序與名稱解析。發現 W-13 和 PHP 不同的地方修在 W-13，下面會列出。

### 分工與資料歸屬

- **`Program` 只有唯讀的類別表**（`vmClassDecl`）：名稱、旗標、成員，以及存放方法本體與常數運算式的函式索引。
- **每次執行自己建立類別**（`vmClass` 與 W-13 的 `class`）。宣告指令執行時，或 `{main}` 開始前（PHP 提早綁定的類別），才從類別表建出來，登錄在當次請求的 heap。物件、static 屬性、類別常數求值後的值、條件式宣告選到的那個類別，都只存在於這次執行。`Program` 沒有任何欄位指向它們，同一個 `Program` 由 16 個 goroutine 同時執行互不相見（`TestW17Stage7StateBelongsToTheRun`，以 `-race` 執行）。
- 類別描述目前沒有跨請求共用。共用可以省掉每次請求重建的成本，但屬性預設值若是常數運算式，要到執行期才知道值，共用的描述就得拆成唯讀與當次兩層。這留給編譯快取的工作包（W-23），列在已知缺口。

### 宣告

- **提早綁定**照 zend_compile_class_decl：頂層、沒有介面也沒有 trait、沒有父類別或父類別已經綁定的類別，在檔案執行前就存在（HIR 的 `Class.Hoisted`，bytecode 的 ClassHoisted）。有 `__toString` 的類別會隱含實作 Stringable，所以不會提早綁定，和 PHP 相同。其他類別編譯成 DeclareClass，執行到才宣告，條件式宣告因此自然成立。
- **繼承規則在宣告當下檢查**（`vm_link.go`，對應 zend_do_link_class）：final、static 與非 static 互換、可見度降低、簽章相容（參數逆變、回傳協變，型別以當時存在的類別判斷）、還沒實作的抽象方法、常數的 final 與來源衝突、trait 的 insteadof／as 與衝突。訊息與行號照 PHP。lowering 對提早綁定的類別已經在編譯期做同樣的檢查；執行期這一套讓條件式宣告、介面與 trait 的類別，以及不是編譯器產生的 bytecode，都受到檢查。
- PHP 8.5 對不是例外的 fatal 也印 stack trace（fatal_error_backtraces）。執行期宣告失敗的 fatal 因此在位置後面接 `Stack trace:`。
- 編譯期才報的三種宣告錯誤（抽象方法有本體、介面方法不是 public、介面宣告屬性）由編譯器回報成 PHP 的編譯錯誤。
- **屬性預設值與類別常數**：字面值直接放進類別表。其他常數運算式編譯成沒有參數的函式，在宣告它的類別範圍裡執行。常數在第一次被讀到時求值並記在當次執行；屬性預設值在類別第一次建立物件或第一次用到 static 屬性時求值（PHP 的 zend_update_class_constants）。

### 呼叫

- **PHP 寫出來的方法呼叫用 VM 自己的 frame**，和函式呼叫一樣，所以 PHP 的遞迴不會加深 Go 的堆疊。frame 多了 `$this`、宣告範圍（`self` 與可見度判斷用）與被呼叫的類別（`static` 用）。
- **runtime 在指令中途回頭呼叫 PHP 時跑巢狀迴圈**（`vmState.invoke`）：解構子、magic method、`__toString`、常數運算式。巢狀深度上限 12,000 層，超過時拋出 PHP 在 C 堆疊用完時的 `Maximum call stack size ... reached. Infinite recursion?`。
- 因為指令執行到一半可能有 PHP 程式進出 frame，frame 物件與它的暫存器視窗改成不會搬動：frame 物件依深度重複使用，三個堆疊（值、位置、元素）滿了就接新的區塊而不是整塊搬家。
- **被呼叫者要到執行期才知道**，所以引數的送法由被呼叫函式的宣告決定（PushAuto、PushTemp、PushResult），對應 PHP 的 SEND_VAR_EX、SEND_VAL_EX、SEND_VAR_NO_REF_EX。
- **`new`** 照 ZEND_NEW：先建立物件，再呼叫建構子。沒有建構子時引數仍然求值後丟棄。
- **型別檢查在被呼叫函式裡做**，和 ZEND_RECV 一樣：接收參數的指令檢查它的型別，所以前面的參數已經進了變數、後面的預設值還沒求值，backtrace 與錯誤的行號因此和 PHP 相同。轉換規則用 W-13 的 `coerceTypedValue`，頂層函式和方法走同一套。strict_types 看呼叫端的檔案，回傳值看函式自己的檔案。runtime 自己發起的呼叫一律是 weak。
- 引數的 TypeError，PHP 在訊息後面接 `called in %s on line %d`，未捕捉時位置寫成 `and defined in`，這裡照做。

### 屬性

- 讀寫、`isset`、`unset`、取參照都呼叫 W-13 的操作。
- 寫入目標的求值順序照 zend_delayed_compile_var：先算目標裡的運算式，再算右邊，最後才逐層取目標。`$o->a->b = v`、`$o->a[k] = v`、`$a[k]->p = v`、`A::$s[k] = v` 這類混合路徑，用 FetchProp、FetchStatic 與原有的 FetchDim 接成一段取值序列，受元素位置規則保護。
- 複合指定與 `++`／`--` 拆成讀取、運算、寫入三條指令，讀取那一條帶著用途，容器不是物件時拋出 PHP 對應的錯誤。屬性不存在時，PHP 先發動態屬性的 Deprecated 再發 Undefined property，順序照做。
- **走訪物件**傳值時逐一讀取目前的屬性，迴圈中加入或改掉的屬性看得到，和 PHP 相同。傳參照時每個屬性變成參照。可見度以迴圈所在的範圍判斷。

### 生命週期

- 解構子不靠 Go finalizer。暫存器裡的物件一沒人用就釋放，解構子才會在 PHP 的時機執行。
- **超大函式也一樣**：原本暫存器數乘指令數超過上限的函式不做活性分析，暫存器要等被覆寫才釋放，陣列看不出差別，物件的解構子卻會延後。現在這種函式改用逐暫存器、記憶體用量和程式碼成線性的分析（`vmLivenessSparse`）。`TestW17Stage7LargeFunctionDestructorTiming` 把解構時機的 fixture 塞進這種大小的 `{main}`，輸出仍和 PHP 相同；把新的分析關掉，其中三個會失敗。
- 沒被使用的呼叫結果由編譯器加 Drop 立刻釋放（PHP 的 ZEND_FREE）。
- **請求結束**時 `{main}` 的變數交給執行框架當成全域符號表，由 W-14 照 PHP 的順序處理：只被全域變數持有的物件依相反順序解構，其餘物件再依建立順序解構。static 變數、static 屬性與類別常數是之後才釋放的儲存空間，不會搶在前面。

### 效能

熱路徑的候選做法都在同一個測試執行檔裡量過才選定。

| 項目 | 採用 | 量到的結果 |
|---|---|---|
| 屬性讀寫 | 每條指令一個當次執行的 inline cache：記下物件的類別、所在範圍與屬性的位置。命中時直接讀寫宣告屬性的位置，條件不符就走 W-13 的完整操作 | `$o->n = $o->n + 1` 的迴圈每圈由約 428 ns 降到約 186 ns |
| 方法查找 | 同一個 cache 記下解析到的方法 | 省掉每次呼叫的名稱轉小寫與查表 |
| 呼叫的附帶資料 | 只有具名引數或略過的參數才配置 | 方法呼叫由每次 96 B、1 次配置降到 0 |
| frame | 逐欄位寫入、指標沒變就不寫 | 整個結構一次指定會經過 GC 的 bulk write barrier，`fib` 比 main 慢 10.6%。改完後慢 3.2%，兩次審查的修正後再量是快 2.0% |

cache 只存當次執行的類別描述指標與位置編號，不存值，也不放進 `Program`。分派迴圈多了二十個 case。和 main（1374780）交替各跑 6 輪 `BenchmarkVMDispatch/*/switch`，兩次審查修正後的最後一輪，五個工作量都比 main 快：算術 16.4%、分支 13.2%、陣列 7.9%、字串 2.5%、遞迴的 `fib` 2.0%，6 輪的範圍不重疊。原始輸出在 [stage7a/bench-main.txt](../implementation/evidence/W-17-vm/stage7a/bench-main.txt)、[stage7a/bench-stage7a.txt](../implementation/evidence/W-17-vm/stage7a/bench-stage7a.txt) 與 [stage7a/bench-objects.txt](../implementation/evidence/W-17-vm/stage7a/bench-objects.txt)。

### 修在 W-13 與 lowering 的地方

- `__get` 回傳的值若對應一個被 `unset` 的 typed 屬性，PHP 會檢查它能不能指定給那個型別，不行就拋出 `Value of type ... returned from ...::__get() must be compatible with unset property ...`。W-13 原本直接回傳（`object_properties.go`）。
- 物件轉成 int 或 float 時發出 `Object of class X could not be converted to int`，結果是 1（`ops_php8512.go`）。
- 透過參照指定時，strict 與否改由 VM 傳入實際的模式，原本一律當成 strict。
- lowering 的 `Access level to ... must be public (as in class X)`：父類別是 public 時 PHP 不加 `or weaker`。
- 函式隱含的 return 改用右大括號那一行（frontend 記下區塊的結束行）。這原本列在已知缺口，`never` 與「沒有回傳值」的 TypeError 讓它看得出差別。

### 獨立審查

審查者（內建的 Opus，全新的脈絡）只拿到需求與程式碼。他寫了 488 個 PHP 探測程式，全部在 arm64 映像對照，另有 50 個在 386、21 個在 amd64。他也用 32 個 goroutine 同時執行同一個 `Program` 檢查隔離（`-race`），手寫約 45 個惡意 bytecode 映像，並測試取消與深層巢狀。請求之間沒有外洩，驗證器沒有被繞過的 Go panic。找到的問題如下，都已修正並補上修正前會失敗的回歸測試，修正前的輸出在 [stage7a/red-review-fixes.txt](../implementation/evidence/W-17-vm/stage7a/red-review-fixes.txt)。

- **請求結束時 panic**（P0）。一個全域物件持有另一個全域物件（`$a->o = $b`）時，關機流程在移除元素後用舊的索引讀符號表，Go panic 直接傳到宿主。這段是 W-06 的程式，物件接上 VM 後才走得到。現在每次都重新檢查索引（`execution_destruct.go`、`heap.go`）。
- **解構子裡建立同類別物件會撐爆 Go 堆疊**（P0）。巢狀深度原本在 frame 收掉之前就減掉，釋放 frame 的值又觸發解構子，深度永遠數不到。現在算到這次呼叫的 frame 都釋放完為止，上限 12,000 層，超過時拋出 PHP 的 `Maximum call stack size` 錯誤（`TestW17Stage7NestedCallbacksAreBounded`）。
- **關機時的解構子呼叫用了 `global` 的函式會以內部錯誤結束**（P0）。`{main}` 的變數那時已經交給執行框架，`global` 現在改從那張符號表找。
- **runtime 包過的錯誤沒有變成未捕捉的錯誤**（P1）。`__get`、`__toString` 裡拋出的錯誤經過 W-13 的回呼邊界後，少了 `Uncaught` 與 stack trace，還被回報成內部錯誤。
- **`$o->typed = &$var` 與 `A::$typed = &$var` 沒有檢查型別**（P1）。現在由 PropSet／StaticSet 的 MemberBindRef 模式處理，值先依屬性的型別檢查，參照再登記為這個屬性的來源。
- **在 `?int` 這類屬性裡自動建立陣列**（P1）。`$o->i[] = 1` 現在拋出 `Cannot auto-initialize an array inside property`。這原本列在已知缺口。
- **建構子沒有完成的物件仍被解構**（P1）。建構子拋出錯誤、`exit`，或引數求值失敗時，PHP 不會呼叫這個物件的解構子，現在照做。這也原本列在已知缺口。
- **只被呼叫引數持有的物件太早解構**（P2）。未捕捉錯誤的 backtrace 持有每個 frame 的引數，PHP 因此先印錯誤才解構它們。現在錯誤回報後才釋放。
- **可見度與訊息**（P2）：兄弟類別重新宣告的 protected 屬性可以從共同祖先的其他子類別存取（W-13 改用最初宣告它的類別判斷）。private 常數不被繼承。`static::m()` 不套用「呼叫範圍自己的 private 方法優先」的規則。`parent::__construct()` 沒有父建構子時是 `Cannot call constructor`。
- **trait 組合的檢查**（P2）：類別方法與 trait 的抽象方法簽章不相容、別名指向不存在的 trait 或方法、同名方法來自兩個 trait 卻沒有指明、trait 與類別的屬性定義衝突，現在都是 PHP 的 fatal。
- **`++`／`--` 超出整數範圍**（P2）：typed int 屬性拋出 `Cannot increment property ... past its maximal value`，原本的訊息是指定 float 失敗，`--` 在最小值甚至沒有錯誤。
- **其他**（P2）：不定參數的型別錯誤在 backtrace 裡把第一個引數印成 NULL。常數運算式的 frame 印成 `[constant expression]()`。`clone($x)` 寫成呼叫的形式、`#[AllowDynamicProperties]`、`foreach (... as $o->p)`、`isset($this)` 原本被拒絕，現在可以執行。宣告和內建類別同名的類別回報 PHP 的 `Cannot redeclare class stdClass`。手寫 bytecode 不能再宣告 stdClass 或 Stringable，型別字串也要通過驗證。
- 審查建議的效能改進採用了一項：`kill` 遇到純量直接略過，不清空也不呼叫釋放。其餘建議（類別描述跨請求共用、動態屬性的快速路徑、`new` 用每個類別的預設值樣板、型別遮罩）列在已知缺口。

審查留下、這一段不修的差異都列在已知缺口。回歸 fixture 共 47 個（`review7a_*` 與 `fatal_review7a_*`）。

#### 第二次審查

Codex 的審查額度在 PR 開出時已用完，依本階段的規則改由另一位獨立審查者（內建的 Opus，全新的脈絡）檢查第一次審查之後的修正（eb00fae..f97a4d7）。他跑了 297 個 PHP 探測程式（arm64 映像）、33 個手寫 bytecode 的執行、32 個 goroutine 各 20 次的隔離與 race 檢查。請求之間沒有外洩，手寫 bytecode 沒有造成 Go panic。找到的問題如下，都已修正並補上修正前會失敗的回歸測試，修正前的輸出在 [stage7a/red-review2-fixes.txt](../implementation/evidence/W-17-vm/stage7a/red-review2-fixes.txt)。

- **釋放很長的物件鏈會撐爆 Go 堆疊**（P0，修正前就有）。一百萬個節點的鏈結串列釋放時，每個節點在 Go 堆疊上多一層，宿主行程直接結束。現在釋放的巢狀超過 512 層時，沒有解構子要跑的物件排進佇列，由最外層依序處理。有解構子的物件仍然巢狀執行，解構子才會在 PHP 的時機跑，深度由巢狀呼叫的上限管。PHP 8.5.12 自己在這個長度會 segfault，所以這項用 Go 測試（`TestW17Stage7LongObjectChainIsFreed`），不是對照 fixture。
- **trait 抽象方法的 `self` 與 `parent`**（P1，第一次修正造成）。`trait T { abstract function m(self $x): self; }` 由類別以 `m(C $x): C` 實作時被當成簽章不相容。trait 簽章裡的 `self` 與 `parent` 現在以使用它的類別解析。static 不一致時改用 PHP 的 `Cannot make static method T::m() non static in class C`。
- **typed 屬性的自動建陣列檢查套用得太廣**（P2，第一次修正造成）。PHP 只在為了寫入元素而取屬性時檢查，`$o->i[0] .= "x"` 與 `$o->i[0]++` 不在其列。屬性被 `unset` 而類別有 `__get` 時先走 `__get`，readonly 屬性先報它自己的錯誤。`false` 也算會被自動轉成陣列的值。未初始化的 typed 屬性做複合寫入時，報 `must not be accessed before initialization`。
- **`++` 的範圍錯誤**（P2，第一次修正造成）。`int|string` 屬性裡的數字字串加到變成 float 是普通的指定，只有原本是 int 的值超出範圍才報錯。
- **引數求值失敗的 `new`**（P2）。類別沒有建構子時，`new A(nope())` 的物件也算建構失敗，不呼叫解構子。
- **關機時的 `global` 看不到宿主給的全域變數**（P2，第一次修正造成）。`{main}` 沒有用到的名稱，現在和執行期間一樣從宿主的值建立。
- **protected(set) 的檢查只修了讀取**（P2）。寫入、`unset`、取參照與 `++` 現在也以最初宣告屬性的類別判斷。
- **`Cannot call constructor`**（P2）。任何類別參照（`A::`、`self::`、`static::`、`parent::`）以字面的 `__construct` 呼叫沒有建構子的類別都報這個錯誤，而且先於 `__call` 與 `__callStatic`。名稱是變數時則是 `Call to undefined method`。private 的父類別建構子報 `Cannot call private A::__construct()`。
- **透過參照寫入 typed 屬性一律用嚴格模式**（P2，修正前就有）。`$o->i = &$arr[0]; $arr[0] = "3";` 在沒有 `strict_types` 的檔案應該轉成 int。陣列元素、`foreach` 與參照的寫入現在帶著寫入檔案的模式。
- **其他**（P3）：對 null 綁定屬性參照的訊息是 `Attempt to modify property`。`empty($this)` 在 static 方法裡回傳 true。父類別的 private 常數不再和 interface 的同名常數衝突。backtrace 持有的引數包含陣列，陣列裡的物件也在錯誤回報後才解構。backtrace 持有的值在錯誤沒有被回報時也會釋放。

第二次審查留下、這一段不修的差異列在已知缺口。回歸 fixture 共 39 個（`review7b_*` 與 `fatal_review7b_*`）。

測試程式加到 732 個（新增 262 個 `class_*`、`object_*`、`method_*`、`prop_*`、`type_*`、`magic_method_*` 與對應的 `fatal_*`，以及兩次審查的 86 個回歸 fixture），三個 profile 各 2,196 項全部和 PHP 8.5.12 逐位元組相同。

### 公開介面沒有變

執行期宣告類別失敗是 PHP 的 fatal，但 `PHPErrorKind` 沒有對應的種類，`Diagnostic` 也沒有欄位放 fatal 後面的 stack trace。改公開介面要先經專案負責人同意，所以這一段先以 `CompileError` 回報這種錯誤，log 管道的那一筆不含 trace。兩者都列在已知缺口，等決定。

## 階段七 B

第七階段的第二段：closure、`use`、箭頭函式、first-class callable 與 `Closure` 類別的方法。格式升到 1.8，新增四個指令（Closure、BindLexical、CallableConvert、ArgByRef）、Return 的一個 Aux、函式的兩個旗標與捕捉清單。四個指令都沒有現有指令能組合出來：沒有指令能建立帶函式索引的物件、把變數交給另一個物件持有、把已開始的呼叫換成值，或在送出引數前查詢被呼叫者的宣告。1.0 到 1.7 的檔案照樣能載入，其中出現 1.8 的內容時以 `ErrInvalid` 拒絕並指出需要 1.8。操作數規則在 [W-16 文件](W-16-bytecode.md)。

### 資料歸屬

- **`Program` 只有 closure 的函式本體與捕捉清單**。本體是帶 FuncClosure 的函式，捕捉清單記下每個 `use` 變數進入哪個 local、傳值還是傳參照。兩者唯讀，可以由多個 goroutine 共用。
- **Closure 物件屬於當次執行**。它是 W-13 的物件（類別 `Closure`：final、不能 `new`、不能有動態屬性），另外帶一份 closure 資料：函式、綁定的 `$this`、類別範圍、被呼叫的類別、捕捉到的值、closure 自己的 static 變數。這份資料和物件同一次配置，隨物件釋放。`Program` 沒有任何欄位指向它。`TestW17Stage7BStateBelongsToTheRun` 以 16 個 goroutine 各 50 次同時執行同一個 `Program`（`-race`），每次的 static 計數、傳參照捕捉與綁定的物件都從頭開始。
- **closure 的 static 變數放在 Closure 物件裡**，同一個宣告建立的兩個 closure 各有一份。由函式或方法做成的 closure（`f(...)`、`Closure::fromCallable`）沒有自己的一份，和原本的函式共用，所以 `f(...)` 與直接呼叫 `f()` 看到同一個計數。

### 建立與捕捉

- **建立**照 ZEND_DECLARE_LAMBDA_FUNCTION：closure 帶走目前 frame 的類別範圍與被呼叫的類別。frame 有 `$this` 而且 closure 不是 static 時綁定 `$this`。
- **捕捉**照 ZEND_BIND_LEXICAL，每個 `use` 變數一條 BindLexical，行號是那個變數所在的行，所以 `use` 換行寫時未定義變數的警告報在 `use` 那一行。傳值捕捉在建立當下複製值並發出未定義的警告。傳參照捕捉把變數變成參照，未定義時建立 null、不警告。
- **箭頭函式**的自動捕捉由 lowering 照 find_implicit_binds 算出，外層變數未定義時不警告，closure 裡的變數保持未定義，要到讀取時才警告。PHP 會把只在內層箭頭函式出現的名稱也列進外層的清單（例如內層的參數），這些名稱在外層沒有變數，編譯器略過它們。
- **進入 closure** 時，frame 直接從 Closure 物件拿到捕捉的值：傳值的複製一份，傳參照的綁上同一個參照。清單在編譯時決定，進入時是一個固定長度的迴圈，不查名稱也不多跑指令。
- **複製**（`clone`、`bind`、`bindTo`）照 zend_array_dup 處理變數：值多一個持有者，沒有其他持有者的參照換成它的值。所以外層變數還在時，複本和原本共用傳參照捕捉；外層變數已經 `unset` 時，複本拿到獨立的一份。static 變數也照這條規則複製目前的值。

### 呼叫

- closure 的呼叫走七 A 的路徑：InitMethod（callable）記下被呼叫者，引數依被呼叫函式的宣告送出，CallPending 進入 frame。frame 多一個欄位持有 Closure 物件，回傳時在變數與 `$this` 之後釋放（zend_leave_helper 的順序），所以執行中把最後一個指向 closure 的變數蓋掉，closure 也活到這次呼叫結束。
- **callable 的值**在同一處解析（`resolveCallable`）：Closure 物件、有 `__invoke` 的物件、函式名稱字串、`'A::m'`、`[$o, 'm']`、`['A', 'm']`，包含 `__call` 與 `__callStatic` 的退路。七 A 的已知缺口「透過字串或陣列呼叫方法」因此補上。錯誤訊息分兩套：動態呼叫照 ZEND_INIT_DYNAMIC_CALL，`Closure::fromCallable` 照 zend_is_callable_ex。
- **first-class callable** 照 PHP 的編法：先像一般呼叫那樣開始（InitCall、InitMethod、InitStatic），再由 CallableConvert 把這個還沒送引數的呼叫換成 Closure。找不到函式、方法不可存取等錯誤因此在建立 closure 的那一行拋出，不是等到呼叫。對 closure 寫 `$f(...)` 得到同一個物件。
- **把元素或屬性傳給傳參照的參數**：被呼叫者要到執行期才知道，所以編譯器先用 ArgByRef 問「這個位置要不要參照」（ZEND_CHECK_FUNC_ARG），再分支成以寫入方式取元素並送出參照，或讀值送出。兩條分支只會執行一條，元素裡的運算式不會求值兩次。這補上七 A 的已知缺口，具名引數與含 nullsafe 的元素仍照舊。

### `Closure` 的方法

`bind`、`bindTo`、`call`、`fromCallable` 與 `__invoke` 由 VM 自己實作，不經過 PHP 的 frame。

- **能不能綁定**照 zend_valid_closure_binding，依序檢查並發出 PHP 的警告後回傳 null：static closure 不能綁物件、方法做成的 closure 不能綁到不相容的類別或解除 `$this`、用到 `$this` 的 closure 不能解除 `$this`、不能把範圍設成內建類別、函式或方法做成的 closure 不能換範圍。
- 綁了物件卻沒有範圍時，範圍是 `Closure` 類別本身，和 PHP 相同。
- **`call`** 不建立新的 closure：同一個 Closure 物件的函式以新的 `$this` 與它的類別當範圍執行，static 變數與捕捉的值和原本的 closure 共用。具名引數對應 closure 自己的參數。`call` 以傳值收引數，closure 的參數若是傳參照，發出 `must be passed by reference, value given` 的警告後照值傳。
- 引數個數或型別不對時拋出的錯誤，backtrace 多一格 `Closure::bind(...)` 或 `Closure->bindTo(...)`。經由 `call` 或 `__invoke` 進入的 closure，它的 frame 在 backtrace 裡照 PHP 顯示成由內部函式呼叫（`[internal function]`），下面接 `Closure->call(...)` 或 `Closure->__invoke(...)` 那一格；引數個數與型別的錯誤訊息也不帶呼叫位置。
- 這些方法本身也能當 callable：`Closure::bind(...)`、陣列 `[$f, 'bindTo']` 接 `(...)`、`'Closure::bind'`。具名引數對應各方法的參數名稱（`closure`、`newThis`、`newScope`、`callback`）。`newScope` 收到 int、float 或 bool 時，在非 strict 的檔案照 PHP 轉成字串當類別名稱。
- `Closure::getCurrent()`（PHP 8.5 新增）回傳目前正在執行的 closure，不在 closure 裡時拋出 `Current function is not a closure`。

### 生命週期

- 捕捉的值、綁定的 `$this` 與 static 變數都是 Closure 物件的持有者，循環回收會沿著它們走（zend_closure_get_gc）。`$this->cb = function () { ... $this ... }` 形成的循環因此要等回收或請求結束才解構，沒有循環的 closure 一被丟掉，捕捉的物件立刻解構。不靠 Go finalizer。
- 暫存器裡的 closure 用完就釋放。`TestW17Stage7BLargeFunctionClosureLifetime` 把六個生命週期的 fixture 塞進大到要用逐暫存器活性分析的 `{main}`，輸出仍和 PHP 相同。

### 其他照 PHP 的地方

- closure 的名稱照 PHP 8.4 之後的寫法：`{closure:/w/cases/x.php:3}`、`{closure:f():3}`、`{closure:A::m():3}`，closure 裡的 closure 以外層 closure 的名稱當前綴（`{closure:{closure:A::m():3}:4}`）。lowering 原本一律往外找到具名函式，已修正。
- 兩個不同的 Closure 物件只有在都是由同一個函式或方法做成、綁定的物件與類別也相同時才相等，其他情況不可比較（zend_closure_compare）。`(array) $closure` 得到只有它自己一個元素的陣列。
- `fn &() => 運算式` 回傳的不是變數時，每次呼叫發出 `Only variable references should be returned by reference`。一般函式的回傳參照仍不支援。
- 型別宣告 `callable` 現在認得程式自己宣告的函式名稱。W-13 的檢查原本看不到 VM 的函式表，一律當成不是 callable。

### 獨立審查

測試案例與審查都照派工規則逐層嘗試。OpenCode 的指令在這台機器上失敗，Copilot 的月額度用完，agy 的 Opus 額度用完，Codex 沒有符合條件的模型，所以測試案例由內建的 Haiku 撰寫（每個檔案都在固定映像跑過，再由主要作者逐一對照），審查由內建的 Opus 在全新的脈絡進行，只拿到需求與程式碼。

審查者跑了 548 個 PHP 探測程式（arm64 映像），另外檢查了手寫的惡意 bytecode、16 個 goroutine 的隔離與 race。請求之間沒有外洩，沒有 Go panic，沒有 P0。找到的問題如下，都已修正並補上修正前會失敗的回歸測試，修正前的輸出在 [stage7b/red-review-fixes.txt](../implementation/evidence/W-17-vm/stage7b/red-review-fixes.txt)。

- **closure 當屬性預設值以內部錯誤結束**（P1，這次造成）。`class A { public $p = static function () {}; }` 原本在 `Compile` 被拒絕，closure 能編譯之後走到執行期，預設值是物件而無法存成快照。現在 `Compile` 帶位置拒絕，列在已知缺口。類別常數、全域 `const` 與參數預設值裡的 closure 可以執行。
- **沒有類別範圍時 `self`／`parent`／`static` 的訊息**（P2）。`::class` 是 `Cannot use "self" in the global scope`，其他用法是 `Cannot access "self" when no class scope is active`，原本兩種都印成另一句。`$x instanceof self` 的左邊不是物件時 PHP 不去找類別，結果是 false，原本拋出錯誤。這些在 closure 之前走不到。
- **`Closure::fromCallable` 不認得相對名稱與呼叫者的 `$this`**（P2）。`'self::m'`、`'static::m'`、`'parent::m'` 現在以呼叫者的範圍解析並發出 `Use of "self" in callables is deprecated`。在物件方法裡以 `'A::m'` 指名非 static 方法時，呼叫者的 `$this` 跟著過去。
- **範圍內的寫法到執行期才被拒絕**（P2）。對 `__call` 做成的 closure 呼叫 `call`、以 callable 指名 `Closure` 的方法、對這些方法用具名引數、`Closure::getCurrent()`，現在都能執行。
- **執行期才知道名稱的內建函式被當成未定義**（P2）。`$f = 'strlen'; $f(...)` 與 `Closure::fromCallable('strlen')` 原本拋出 PHP 的 `Call to undefined function`，看起來像程式自己的錯。現在以 Unsupported 結束。`callable` 型別接受內建函式的名稱，呼叫時才拒絕。這個問題在 main 的 `$f('abc')` 就有。
- **`Closure` 方法的 backtrace 沒有持有引數**（P2）。`Closure::fromCallable(new D)` 失敗時，D 的解構子在錯誤訊息之前就執行。現在 backtrace 的每一格都持有它的引數，釋放順序也照 PHP：先是 `call` 的物件，再是引數。
- **驗證器讓 closure 函式當類別常數或屬性的初始值函式**（P2）。手寫 bytecode 可以不經 Closure 物件直接進入 closure 的本體。現在類別成員的任何函式都不能是 closure 函式。
- **其他**（P3）：抽象方法以 callable 指名時先報 `Cannot call abstract method`。捕捉清單不能指向參數的 local（載入時拒絕，因為驗證器看不出參數存進哪個 local）。CallableConvert 不能接在 `new` 開始的呼叫後面。Closure 類別只在當次執行第一次建立 closure 時登記，不再每次查表。`$f($a[])` 這類把 `[]` 傳給執行期才知道的被呼叫者，原本是編譯器的內部錯誤（main 上的方法呼叫也是），現在 `Compile` 帶位置拒絕。

審查留下、這一段不修的差異都列在已知缺口。回歸 fixture 共 20 個（`closure_review_*`、`fatal_closure_review_*`、`fatal_callable_review_*`），另有 `internal/bytecode/format18_test.go`、`internal/runtime/vm_closure_test.go` 與 `internal/vm/compile_test.go` 的 Go 測試。CallableConvert 接在建構子呼叫後面的檢查沒有獨立的回歸測試：測試用的組譯器不能宣告類別，這一項只有審查者以引擎編譯再改寫映像的探測（`~/.cache/phego-work/review7b-1/tools/zz_hostile_test.go` 的 H4）。

測試程式加到 863 個（新增 131 個：82 個七 B 的 fixture、29 個自我探測、20 個審查的回歸 fixture），三個 profile 各 2,589 項全部和 PHP 8.5.12 逐位元組相同。

### 效能

候選做法在同一個測試執行檔量過，原始輸出在 [stage7b](../implementation/evidence/W-17-vm/stage7b)。量測都在本機（Apple M3）進行，只用來比較同一輪裡的做法，不是對外的效能數字。

| 項目 | 採用 | 量到的結果 |
|---|---|---|
| closure 資料的配置 | 和物件同一次配置 | 另外配置時建立一個 closure 是 3 次配置，合併後 1 次。十萬次建立由約 37.6 ms 降到 32.1 ms，帶三個捕捉的由約 48.3 ms 降到 42.3 ms（`bench-create-separate.txt` 對 `bench-create-shared.txt`） |
| 捕捉的值進入 frame | 進入時依編譯好的清單直接複製，不查名稱也不多跑指令 | 三個傳值捕捉的 closure 每次呼叫約 391 ns，沒有捕捉的約 301 ns，差的 90 ns 包含本體多出的三次讀取與加法。同樣三個值改用引數傳給一般函式，每次多約 151 ns（332 ns 對 182 ns） |
| closure 的呼叫 | 沿用七 A 的動態呼叫路徑，以物件上的一個欄位認出 Closure | 每次約 301 ns，和方法呼叫（約 285 ns）同級，比直接呼叫函式（約 182 ns）多約 120 ns。每次呼叫沒有配置。建立一個 closure 約 307 ns |

分派迴圈多了四個 case，Return 多一個 Aux 的判斷，進入 frame 時多看 closure 的兩個欄位。和 main（4c64be5）交替各跑 6 輪 `BenchmarkVMDispatch/*/switch`，中位數是算術 −2.1%、分支 −1.7%、字串 −0.2%、陣列 −1.2%、遞迴的 `fib` +0.8%，前兩項的 6 輪範圍不重疊，其餘重疊。七 A 的物件工作量（`BenchmarkW17Stage7`，各 3 輪）則慢 1.0% 到 1.9%：函式呼叫 +1.2%、方法呼叫 +1.0%、static 呼叫 +1.9%、建立物件 +1.5%，屬性讀寫 −0.5%，範圍大多不重疊。這是每次呼叫多出的欄位判斷與建立物件時多一次類別比較的成本，沒有再壓低。原始輸出在 `bench-main.txt`、`bench-stage7b.txt`、`bench-objects-main.txt` 與 `bench-objects-stage7b.txt`。

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
- **兩個元素之間參照指定的運算式值**：`$x = ($a[0] = &$b[0])` 在 `Compile` 回報不支援。單獨成為陳述式的 `$a[0] = &$b[0];` 可以執行。
- **從元素解構參照的運算式值**：`$y = ([&$a] = $x[0])` 在 `Compile` 回報不支援；`$y = ([&$a] = $x)` 與單獨成為陳述式的寫法可以執行。
- **`[&$a] = f()`**：PHP 會發出 `Attempting to set reference to non referenceable value` 的 notice 再照值讀取；Phego 沿用第三階段「參照到函式呼叫結果」的不支援回報。執行期 FetchDim 的容器暫存器不是參照時也以 Unsupported 結束，編譯器不會產生這種 bytecode。
- **沒有參數表的函式不受參照檢查**：格式 1.6 的驗證器只在被呼叫函式記錄了參數表時，拒絕以 Call 把參照送給傳值參數。編譯器產生的函式都有參數表，手寫的 bytecode 可以省略，舊版檔案則都沒有。被呼叫端的 Store 會解開參照，不會造成記憶體問題。
- **`$GLOBALS` 與 `global $$name`**：`Compile` 回報不支援。全域變數目前只能以編譯時已知的名稱取得。
- **展開物件與 Traversable**：`f(...$iterator)` 要等迭代器介面，執行期遇到物件以 Unsupported 結束。
- **`func_get_args()`、`func_num_args()`**：內建函式，屬於模組工作包。
- **把不是變數的值以名稱或不定參數傳給傳參照參數**：`f(r: 1 + 2)` 與 `f(r: g())` 沿用第三階段的不支援回報。PHP 對字面值是執行期的 `Error: f(): Argument #1 ($r) could not be passed by reference`，對函式回傳值是 `Only variables should be passed by reference` 的 notice。
- **heredoc 當第一個引數時的引數列行號**：PHP 不認得被呼叫函式時，後面單純變數引數的警告報在 heredoc 內容的那一行，Phego 報在 heredoc 開頭那一行。HIR 的字面值只記開頭的行。
- **引數列行號由下一個 token 決定的情況**：PHP 不認得被呼叫函式，而且第一個引數是 magic constant，或是名稱為 `array`、`list`、`fn`、`function`、`match` 的具名引數時，若後面的逗號或冒號換到下一行，PHP 的行號是下一行，Phego 是 token 自己那一行。原因推測是 PHP 的 parser 要多讀一個 token 才建立這些節點，還沒有讀原始碼確認。
- **函式宣告和呼叫寫在同一行、而且呼叫在前**：HIR 沒有欄位資訊，同一行的宣告一律視為在呼叫之前。`f(\n$u); function f() {}` 的前半段若和宣告擠在同一行，警告的行號會和 PHP 不同。
- **`error_reporting` 的 INI 值只接受十進位整數**：PHP 的 `-d` 也接受 `E_ALL` 這類運算式。
- **依 CPU 架構而異的預先定義常數**：CHAR_MAX（arm64 為 255，amd64 與 386 為 127）與 PHP_BUILD_DATE 在 `Compile` 回報不支援；profile 要有架構欄位才能給值。
- **STDIN、STDOUT、STDERR**：值是資源，`Compile` 回報不支援，等資源與串流的工作包。
- **預先定義常數上的運算只在常數運算式與 match 條件裡折疊**：PHP 把一般運算式裡的 `PHP_INT_MAX + 1` 也折成常數，Phego 在執行時算，結果相同，目前沒有觀察到可見的差別。
- **含預先定義常數的參數預設值的警告行號**：`function f($a = "5x"\n + PHP_INT_SIZE)` 的 `A non-numeric value` 警告 PHP 報在 PHP_INT_SIZE 那一行，Phego 報在參數的行；折疊時移動 CG(zend_lineno) 的機制還沒查清楚。
- **以執行期才算出的值為值的使用者常數當參數預設值**：PHP 對不計數的結果才快取預設值；Phego 把常數名稱的預設值一律當成可快取的純量。目前能寫出的常數值都在編譯期折成字面值，看不出差別。
- **`#[\Deprecated]` 等常數屬性**在 `Compile` 回報不支援。
- **match 沒有 jump table**：PHP 對至少兩個整數或字串字面值的條件用 hash 查表，Phego 依序比較，條件很多時較慢，結果相同。
- **`define()`、`constant()`、`defined()`**：內建函式，屬於模組工作包；`const` 以外沒有辦法在執行期宣告常數。
- **執行期 fatal 的公開介面**：宣告類別失敗的 fatal 以 `PHPError` 的 `CompileError` 種類回報，`Diagnostic` 沒有欄位放 fatal 後面的 stack trace，所以 log 管道的那一筆只有訊息與位置。display 管道的文字是完整的。加一個 `FatalError` 種類與 `Diagnostic.Trace` 欄位是只增加的公開介面變更，等專案負責人決定。對照測試在 log 模式自己補上 `#0 {main}`，fixture 只在頂層觸發這種 fatal。
- **類別描述每次執行重建**：`Program` 只共用類別表，W-13 的類別物件每次請求從頭建立。宣告很多類別的程式每次請求都付這個成本，等編譯快取的工作包（W-23）把描述拆成唯讀與當次兩層。
- **以名稱把元素或屬性傳給執行期才知道的被呼叫者的傳參照參數**：`$o->m(r: $a[0])` 與含 nullsafe 的 `$o->m($p?->q)` 遇到傳參照的參數以 Unsupported 結束，不會把複本交給函式。位置引數（`$o->m($a[0])`、`$f($p->q)`）在七 B 起照 PHP 處理。
- **把 `$a[]` 傳給執行期才知道的被呼叫者**：`$f($a[])`、`$o->m($a[])` 在 `Compile` 回報不支援。PHP 在參數傳參照時新增元素，否則拋出 `Cannot use [] for reading`，目前沒有指令能在執行期拋出這個錯誤。
- **兩個不同型別的 typed 屬性綁在同一個參照**：`$a->i = &$a->s`（一個 int、一個 string）PHP 以 `Reference with value of type ... is not compatible with property ...` 拒絕。Phego 只檢查值能不能指定給目標屬性，之後寫入才報出不同的錯誤。透過參照 `++` 一個 typed int 屬性超出範圍時，訊息也和 PHP 不同。
- **`??=` 的目標是屬性底下的元素、而且運算元有副作用**：`$o->p[f()] ??= 1` 在 `Compile` 回報不支援。PHP 把目標編譯兩次但共用運算元的結果，這裡只在運算元是變數與字面值時才這樣做。
- **closure 當屬性預設值**：`public $p = static function () {};` 與 static 屬性的同樣寫法在 `Compile` 回報不支援。屬性預設值目前以快照保存，放不了物件。類別常數、全域 `const` 與參數預設值裡的 closure 可以執行。
- **循環回收不會在候選數量到門檻時自動啟動**：PHP 在可能的循環根累積到 10,001 個時回收一次，Phego 只在請求結束時回收。大量 `$this` 與 closure 互相持有的物件，解構子因此都排到 `end` 之後。這在 main 的一般物件循環就是如此，沒有內建函式能手動觸發。
- **callable 裡的 `parent::m` 形式**：`['B', 'parent::m']`、`[$this, 'parent::m']` 與 `'B::parent::m'`，PHP 發出棄用通知後呼叫父類別的方法（最後一種是 `Class "B::parent" not found`），Phego 當成方法名稱而找不到。動態呼叫裡的 `'self::m'`、`'parent::m'`、`'static::m'` 字串也還不認得，只有 `Closure::fromCallable` 認得。
- **`Closure::call` 釋放引數的順序**：PHP 先釋放新的 `$this` 再釋放引數，Phego 相反。只有兩者都只剩這一個持有者、而且都有解構子時看得出來。未捕捉錯誤時的順序已照 PHP。
- **對 `Closure` 方法做成的 closure 呼叫 `call`**：`$b = $f->bindTo(...); $b->call($o)` 這類寫法以 Unsupported 結束。對一般 closure 與 `__call` 做成的 closure 呼叫 `call` 可以執行。
- **執行期才知道名稱的內建函式到執行期才拒絕**：`$f = 'strlen'; $f('abc')`、`Closure::fromCallable('strlen')` 與 `callable` 型別的 `'abs'` 在執行到時以 Unsupported 結束，之前的輸出已經寫出。名稱是執行期的值，`Compile` 看不到。
- **箭頭函式結束的行**：PHP 把行號計數器停在箭頭函式運算式結束的那一行，也就是 parser 多讀的下一個 token 所在的行。Phego 取運算式裡最大的行號，下一個 token 換行時會差一行，只影響同一個陳述式裡後面才發出的警告。
- **逐暫存器活性分析的工作量上限**：超大函式超過上限後不再分析，只被暫存器持有的 closure 要等暫存器被覆寫才釋放。審查在大小限制內做不出會碰到上限的程式（3,000 個陳述式仍準時釋放，更大的被「program too large」拒絕），所以沒有觀察到差別，也還不能排除。
- **closure 的捕捉另外配置**：有 `use` 的 closure 建立時多一次配置放捕捉的值。把少量捕捉放進物件同一塊記憶體是審查提出、還沒做的改進。
- **回傳參照的函式與方法**：在 `Compile` 回報不支援。`fn &() => ...` 可以宣告與呼叫，但 `$r = &$f()` 仍屬「參照到函式呼叫結果」，在 `Compile` 回報不支援，所以還拿不到它回傳的參照。它回傳的是函式呼叫的結果時，一律發出 `Only variable references should be returned by reference`，PHP 只在那個結果不是參照時才發。
- **stdClass 與 Stringable 以外的內建類別與介面**：`new ArrayObject`、`extends Exception`、`implements Countable`、`implements ArrayAccess` 等在 `Compile` 回報不支援，不會被當成不存在的類別。名稱清單由固定映像的 `get_declared_classes()` 等產生（`internal/vm/classes_gen.go`）。
- **巢狀呼叫的深度上限是 12,000 層**：解構子、magic method、`__toString` 互相巢狀超過這個深度時拋出 PHP 的 `Maximum call stack size ... reached`。PHP 的門檻取決於 C 堆疊大小，兩邊觸發的深度不同。
- **回報未捕捉錯誤之後才解構的物件，它的解構子再出錯**：`f(new D)` 在 `f` 裡出錯，D 的解構子又拋出錯誤或 `exit` 時，PHP 印出第二個 fatal 或採用 `exit` 的結束碼。Phego 執行解構子，丟掉它的錯誤。未捕捉錯誤之後，關機解構子裡的 `exit("字串")` 把結束碼設成 0，PHP 維持 255。
- **函式結束時釋放區域變數所觸發的解構子，backtrace 多一格**：PHP 那時已經離開被呼叫的函式，Phego 的 backtrace 還帶著它。
- **trait 組合的少數錯誤訊息**：抽象方法由父類別或另一個 trait 滿足時的檢查順序、巢狀 trait 的抽象方法簽章文字、兩個 trait 屬性衝突時先列哪個名稱、`insteadof` 與別名規則的檢查順序，和 PHP 不同。這些都是 fatal，只差訊息。
- **常數運算式裡的 `parent::X`、`self::X`**：存取不到時 PHP 的訊息照寫法印 `parent::X`，Phego 印解析後的類別名稱。
- **類別常數不在第一次 `new` 時求值**：`class A { const Z = UNDEF; } new A;` PHP 在 `new` 時報錯，Phego 到讀取常數時才報。
- **綁定參照到 readonly 或 magic 屬性的檢查順序**：`$o->r = &$x` 對 readonly 屬性會先轉換 `$x` 的值才報錯。對只有 `__get`／`__set` 的屬性，PHP 拋出 `Cannot assign by reference to overloaded object`，Phego 以 Unsupported 結束。
- **`$u->p = &$x` 而 `$u` 未定義**時多一個 `Undefined variable` 警告。
- **private 或 protected 的解構子**：PHP 在範圍外釋放物件時拋出 `Call to private D::__destruct() from global scope`，請求結束時則發出警告後略過。Phego 照樣執行它。
- **自我參照的類別常數**：`const X = self::Y; const Y = self::X;` PHP 的訊息是 `Cannot declare self-referencing constant self::Y`，backtrace 只有一格。Phego 寫成 `A::X`，多一格。
- **`(object)` 轉型的陣列帶有 mangled 的 key**（`"\0A\0p"`）：從類別外走訪時 PHP 看得到三個屬性，Phego 把它們當成 private／protected 而略過。
- **`function &__get`** 與其他回傳參照的 magic method 在 `Compile` 回報不支援（同回傳參照的函式）。
- **動態屬性沒有快速路徑**：inline cache 只處理宣告的屬性，stdClass 與其他動態屬性每次都走 W-13 的完整操作，約慢一倍。`new` 每個物件有三次配置，預設值每次從快照匯入。typed 屬性的快速路徑只認六種單純型別，`?int`、union 與類別型別都走完整的轉換。這三項都是審查提出、還沒做的改進。
- **型別字串只驗證形狀**：手寫 bytecode 可以宣告 static 的 `__construct` 或參數個數不對的 magic method，PHP 的編譯器會拒絕。執行期不會因此出現記憶體安全問題，行為則沒有定義成和 PHP 相同。
- **暫存器釋放會登記 GC 的可能根**：讀取變數裡的物件到暫存器再釋放，會把它登記成循環回收的候選，PHP 讀取變數不動參照計數。大量不同的物件各被讀過一次時，自動回收可能比 PHP 早觸發，只有循環垃圾的解構時機看得出來。
- **走訪物件時 key 一律是字串**：PHP 對整數樣子的屬性名稱給整數 key。
- **簽章相容的判斷是保守的近似**：參數與回傳型別以 union 的成員逐一比對，交集型別（`A&B`）與 `callable`、`iterable` 的特殊規則沒有完整實作，可能拒絕或接受少數 PHP 判斷不同的宣告。
- **`declare(ticks=1)` 與 `declare(encoding=...)`** 編譯成沒有作用。tick 函式要靠內建函式註冊，目前觀察不到差別。
