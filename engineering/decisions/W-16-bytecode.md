# W-16：Phego Bytecode 與載入驗證

需求：REQ-025、REQ-061、REQ-064。局部測試對應 T-049、T-050、T-121、T-122、T-127、T-128。

本文件供編譯器、VM、快取與原生後端維護者使用。W-16 提供程式格式及載入驗證，PHP 執行與完整產品驗收由 [W-17 計畫](https://github.com/HazelnutParadise/phego/blob/b104c91278e756546aa5b60ef3e77b7f6c1f3c82/docs/implementation/W-17.md)、W-23、W-41、W-44 整合。實作與驗證進度以 [W-16](/engineering/implementation/W-16.md) 為準。

## 程式只在驗證完成後發布

```text
編譯器 Image ── Build ──┐
                       ├── 不可變 Program ── Encode ── 獨立 byte slice
快取 bytes ─── Load ────┘
                ↑
     宿主提供 identity、可信摘要、能力與驗證上限
```

`Image` 是建置輸入。`Build` 驗證後取得自己的資料副本。`Load` 將所有輸入視為不可信資料，成功前不交出任何 Program。兩個入口都不得執行 PHP、開啟來源檔案或呼叫宿主 callback。呼叫者在入口返回前須保持輸入不變。

Program 僅保存不可變的編譯資料。暫存器、local、引數、pending completion、靜態變數、環境、session、物件、回饋及失敗狀態由當次 Execution 擁有。取用函式資料的介面只回傳值或唯讀 view，不能取得可修改的 slice。

## 身分、損毀與來源信任分別檢查

`Identity` 以完整欄位相等判斷，包含精確 `profile.Profile`、客體平台規則、目標、來源、編譯器、模組、Runtime ABI、政策及快取隔離範圍。六個摘要欄位必須是規範化 manifest 的 SHA-256，空 manifest 也有非零摘要。

| 欄位 | manifest 必須涵蓋的內容 |
|---|---|
| Source | 來源 bytes 與影響語意的虛擬來源身分 |
| Compiler | 編譯器 build、IR 與格式版本、最佳化設定及影響編譯的 INI |
| Modules | 模組版本、宣告及能力契約 |
| RuntimeABI | Runtime layout 與語意契約版本 |
| Policy | 影響編譯或執行的安全語意與政策版本 |
| Scope | 站台、租戶或宿主指定的快取隔離範圍 |

`GuestPlatform` 明示 PHP 可觀察的平台規則。`Target` 包含產物依賴的目標與 CPU 條件。沒有 CPU 假設的純 Go bytecode 可使用 `portable-go`。平台字串的定義由編譯器及宿主共同維護，loader 比較完整值，不能自行猜測或轉換。

內嵌摘要檢查資料是否損毀。`LoadOptions.TrustedDigest` 則是宿主從獨立可信管道取得的整份產物 SHA-256。若同時從不可信檔案讀出摘要和 bytes，便沒有來源信任。W-16 不包含原生碼區段，遇到未知功能或原生旗標即拒絕，原生簽章政策由後續工作包提供。

## 格式版本與固定表頭

| byte offset | 寬度 | 內容 |
|---|---|---|
| 0 | 8 | `PHEGOBC` 加 NUL |
| 8 | 2 | major，版本 1 |
| 10 | 2 | minor，`Encode` 寫入 2，`Load` 接受 0、1 與 2 |
| 12 | 4 | 表頭大小，必須為 64 |
| 16 | 8 | 整份檔案長度，必須與輸入完全相等 |
| 24 | 32 | SHA-256，計算 `data[:24]` 接 `data[56:]` |
| 56 | 8 | features，目前必須為零 |

所有整數均採 little-endian。解析器拒絕不同 major、大於 2 的 minor、未知旗標、截斷及尾端多出的 bytes。內嵌摘要包含 features；只有摘要欄位本身不參與計算。載入器不嘗試猜測舊版或自動轉換。

## Payload 的區段順序

表頭之後固定依序放置下表區段。`str` 為 `u32` byte 長度接原始 bytes，沒有字元編碼轉換。`table` 為 `u32` 筆數接連續 records。解析器核對筆數、剩餘 bytes 與資源上限後才配置記憶體，所有區段讀完後不得有剩餘資料。

| 區段 | record 的欄位順序 |
|---|---|
| identity | release:str、IntWidth:u32、ShortOpenTag:u8（0 或 1）、GuestPlatform:str、Target:str，再接 Source／Compiler／Modules／RuntimeABI／Policy／Scope 各 32 bytes |
| 程式入口 | Required:u16、Entry:u32 |
| constants table | Kind:u8、Bits:u64、Bytes:str |
| sources table | Name:str、Digest:32 bytes、Bytes:u32、Lines:u32 |
| functions table | Name:str、Params:u32、Locals:u32、MaxStack:u32、ReturnKind:u8，再接以下函式內表格，minor 0 為前四張，minor 1 起為五張 |
| function registers table | Kind:u8 |
| function instructions table | Op:u16、Aux:u16、A:u32、B:u32、C:u32 |
| function positions table | Source:u32、Start:u32、End:u32、Line:u32 |
| function handlers table | Start:u32、End:u32、Target:u32、BodyEnd:u32、StackDepth:u32、Register:u32、Kind:u32 |
| function local names table（minor 1 起） | 常數索引:u32 |

Function 的前 Params 個暫存器必須是 Value，作為已定義的參數。每條指令必須對應一筆 Position。Position 全零表示 synthetic，否則 Source 是從 1 開始的 sources 索引，Line 必須為正，Start／End 為來源 bytes 的半開區間。v1 的 Handler.StackDepth 固定為零，Finally.Register 固定為零，Catch.Register 則指向 Value 暫存器。

RegisterKind 的 Value／Location 分別是 1／2。ConstantKind 的 Null／Bool／Int／Float／String 分別是 1–5。HandlerKind 的 Catch／Finally 分別是 1／2。CapabilitySet 的 HostCallbacks／DynamicCode／FileRead／Shell 分別為 bit 0–3。

Opcode 固定編號為 Checkpoint=1、Const=2、Move=3、Local=4、Load=5、Store=6、Binary=7、Jump=8、Branch=9、Push=10、Call=11、CallValue=12、HostCall=13、Return=14、ReturnRef=15、Throw=16、Output=17、NewArray=18、ArrayGet=19、Include=20、Eval=21、Shell=22、Drop=23、Leave=24、EndFinally=25。格式 1.1 接著加入 Unary=26、Cast=27、IncDec=28、ArraySet=29、ArrayAppend=30、ArrayUnset=31、Isset=32、ArrayIsset=33、Unset=34、Exit=35、ArrayAdd=36。格式 1.2 再加入 FetchDim=37、IterInit=38、IterNext=39、IterKey=40、ParamDefault=41，格式 1.3 再加入 Bind=42、MakeRef=43。零與其他編號均為未知指令，目前第一個未知編號是 44。BinaryOp 的 Add／Subtract／Multiply／Divide／Concat／Equal／Identical／Less／BitAnd／BitOr／BitXor 依序為 1–11，格式 1.1 加入的 Modulo／Power／ShiftLeft／ShiftRight／NotEqual／NotIdentical／LessOrEqual／Spaceship／BoolXor 依序為 12–20。UnaryOp 的 Not／Negate／Plus／BitNot 為 1–4。CastKind 的 CastBool／CastInt／CastFloat／CastString／CastArray 為 1–5。IncDecOp 的 PreInc／PreDec／PostInc／PostDec 為 1–4，前置形式的結果是新值，後置形式的結果是舊值。ArrayWriteFlags 的 ValueLocation／KeyLocation 為 bit 0／1，ArrayAddFlags 的 AddKey／AddKeyLocation 為 bit 0／1。FetchFlags 的 bit 0–1 是模式（FetchWrite=0、FetchReadWrite=1、FetchUnset=2），bit 2 是 FetchKeyLocation，bit 3 是 FetchAppend，bit 4–5 是用途（0 表示接著做下一層元素操作，FetchForIncDec=1，FetchForAssignOp=2，格式 1.3 的 FetchForRef=3 表示元素要被綁成參照，只能搭配 FetchWrite），格式 1.4 的 bit 6 是 FetchContainerValue（容器 B 是存放參照的 Value 暫存器，只能搭配 FetchWrite 與 FetchForRef）。ArrayGet 的 Aux 為 0、1（格式 1.1 的安靜讀取）或 2（格式 1.4 的 FETCH_LIST_R）。IterFlags 的 IterLocation=1，格式 1.3 加入 IterByRef=2。BindFlags 只有 BindValue=1。ArraySet 的 Aux 在格式 1.2 以 bit 3–15 存放結果暫存器：0 表示沒有結果，否則是 Value 暫存器的索引加一，bit 2 必須為零，因此只有索引小於 8191 的暫存器能當結果。

格式 1.1 由 W-17 為第一版 VM 加入，只新增編號與 Aux 值，原有編號不變。`Encode` 一律寫入 minor 1。minor 0 的 1.0 檔案仍可載入，但只要其中出現 1.1 的指令（包含 ArraySet、ArrayAppend、ArrayUnset 與 ArrayAdd 的任何 Aux）、BinaryOp 或 Load／ArrayGet 的 Aux 1，`Load` 就以 `ErrInvalid` 拒絕並指出該指令，這和未知 opcode 的回報方式相同。1.1 的每個函式在例外表後多一張 local 名稱表，筆數為 0 或恰好等於 Locals，每筆指向存放不含 `$` 變數名稱的 String 常數，讓 VM 能印出 PHP 的未定義變數警告並依名稱接收宿主提供的全域變數。minor 0 檔案沒有這張表，載入後沒有名稱。筆數不符、索引超出範圍或常數不是 String 時以 `ErrInvalid` 拒絕並指出函式。

格式 1.2 由 W-17 第二階段加入，payload 的版面和 1.1 相同，只新增五個指令與 ArraySet 的結果暫存器。minor 0 與 minor 1 的檔案照樣能載入，但只要出現 1.2 的指令或帶結果暫存器的 ArraySet，`Load` 就以 `ErrInvalid` 拒絕、指出該指令，並說明它需要 1.2 而檔案宣告的是哪一版。

格式 1.3 由 W-17 第三階段加入，用來表達 PHP 的參照，payload 的版面也不變。新增的是 Bind、MakeRef 兩個指令，IterInit 與 IterNext 的 IterByRef，以及 FetchDim 的 FetchForRef 用途。minor 0 到 2 的檔案照樣能載入，出現上述任一項時，`Load` 以同樣的方式拒絕並指出需要 1.3。

格式 1.4 由 W-17 第四階段加入，用來表達 PHP 的解構，沒有新增指令，payload 的版面也不變。新增的是 ArrayGet 的 Aux 2（ZEND_FETCH_LIST_R）與 FetchDim 的 FetchContainerValue（ZEND_FETCH_DIM_W 的容器是 VAR）。`Encode` 一律寫入 minor 4。minor 0 到 3 的檔案照樣能載入，出現上述任一項時，`Load` 以同樣的方式拒絕並指出需要 1.4。

## 固定指令與純量常數

每條指令為 16 bytes，依序是 `opcode:u16`、`Aux:u16`、`A:u32`、`B:u32`、`C:u32`，採 little-endian。跳躍目標是函式內的指令索引，不能跳進操作數或其他函式。未知 opcode、無效 Aux 與非零的未使用欄位一律拒絕。

常數只有 null、bool、整數、浮點與任意 bytes 字串。浮點保存 IEEE-754 bits，整數須符合客體的 32 或 64 位元範圍。陣列、物件及 reference 在請求內建立，格式沒有 Go 指標、原生位址、宿主 closure 或活躍 resource。

每個暫存器固定為 `Value` 或 `Location`。`Value` 保留 PHP 動態型別，`Location` 表示 Runtime 管理的位置。驗證器檢查類別及所有路徑上的先定義後讀取，不推測 PHP 值必定是整數或物件。PHP 的 undefined value 與驗證器的尚未寫入暫存器是不同狀態。

## 指令操作數

下表省略的操作數必須為零。除 Binary、Include、Load、ArrayGet、Unary、Cast、IncDec、Exit、ArraySet、ArrayAppend、ArrayUnset、ArrayAdd、FetchDim、IterInit、IterNext、IterKey 與 Bind 外，Aux 必須為零。暫存器索引從零開始。

| 指令 | A | B | C |
|---|---|---|---|
| Checkpoint | — | — | — |
| Const | 目的 Value | 常數索引 | — |
| Move | 目的暫存器 | 相同類別來源 | — |
| Local | 目的 Location | local 索引 | — |
| Load | 目的 Value | 來源 Location | — |
| Store | 目的 Location | 來源 Value | — |
| Binary | 目的 Value | 左 Value | 右 Value |
| Jump、Leave | 目標指令索引 | — | — |
| Branch | 條件 Value | true 目標 | false 目標 |
| Push | 引數暫存器 | 0 為位置引數，否則為字串常數索引加一 | — |
| Call | 符合 callee ReturnKind 的目的 | 函式索引 | 引數個數 |
| CallValue | 目的 Value | callable Value | 引數個數 |
| HostCall | 目的 Value | 宿主名稱的字串常數索引 | 引數個數 |
| Return | 回傳 Value | — | — |
| ReturnRef | 回傳 Location | — | — |
| Throw、Output | 來源 Value | — | — |
| NewArray | 目的 Value | — | — |
| ArrayGet | 目的 Value | 容器 Value | key Value |
| Include、Eval、Shell | 目的 Value | 來源 Value | — |
| Drop | 要釋放的暫存器 | — | — |
| EndFinally | — | — | — |
| Unary | 目的 Value | 來源 Value | — |
| Cast | 目的 Value | 來源 Value | — |
| IncDec | 結果 Value | 目標 Location | — |
| ArraySet | 容器 Location | key Value，Aux 有 KeyLocation 時為 Location | 值 Value，Aux 有 ValueLocation 時為 Location |
| ArrayAppend | 容器 Location | 值 Value，Aux 為 ValueLocation 時為 Location | — |
| ArrayUnset | 容器 Location | key Value，Aux 為 KeyLocation 時為 Location | — |
| Isset | 目的 Value | 檢查的 Location | — |
| ArrayIsset | 目的 Value | 容器 Value | key Value |
| Unset | 要 unset 的 Location | — | — |
| Exit | Aux 1 時為狀態 Value | — | — |
| ArrayAdd | 就地加入元素的陣列 Value | 值 Value | Aux 有 AddKey 時為 key Value，同時有 AddKeyLocation 時為 Location |
| FetchDim | 目的 Location（元素位置） | 容器 Location，Aux 有 FetchContainerValue 時為存放參照的 Value | key Value，Aux 有 FetchKeyLocation 時為 Location，有 FetchAppend 時為零 |
| IterInit | 迭代器 Value | 被迭代的 Value；Aux 為 IterByRef 時可以是 Location | 不能迭代時的目標指令索引 |
| IterNext | 迭代器 Value | 元素的目的 Value，Aux 有 IterLocation 時為 Location | 迭代結束時的目標指令索引 |
| IterKey | key 的目的 Value，Aux 為 IterLocation 時為 Location | 迭代器 Value | — |
| ParamDefault | 參數暫存器（索引小於 Params） | 呼叫有傳這個引數時的目標指令索引 | — |
| Bind | 目的 Location | 來源 Location，Aux 為 BindValue 時為 Value | — |
| MakeRef | 目的 Value | 來源 Location | — |

Binary 的 Aux 為 Add 至 BoolXor 的有效列舉值。Include 的 Aux 為 0–3，分別對應 include、include_once、require、require_once。Load 的 Aux 為 0 或 1，ArrayGet 的為 0、1 或 2。1 表示安靜讀取，Load 不發出未定義變數警告，ArrayGet 不發出未定義 key 或 offset 警告。ArrayGet 的 2 是解構的元素讀取（zend_fetch_dimension_address_read 的 is_list）：陣列照一般讀取，缺少的 key 發出警告並讀到 null；容器是 null 時安靜讀到 null；其他型別（字串也算，不當成字串位移）發出 `Cannot use T as array` 並讀到 null，T 是型別名稱（true 與 false 都是 bool）。Unary、Cast、IncDec 的 Aux 分別是 UnaryOp、CastKind、IncDecOp 的有效值。Exit 的 Aux 為 0 時沒有狀態且 A 為零，為 1 時 A 是狀態 Value。

ArraySet 的 Aux 為 ArrayWriteFlags 的組合（0–3），ArrayAppend 的 Aux 為 0 或 ValueLocation。沒有旗標的操作數是指令執行前已讀好的 Value。有旗標的操作數是 Location，由指令在 PHP ZEND_ASSIGN_DIM 讀取該 CV 的時點自行讀取，未定義時發出未定義變數警告並視為 null：先檢查容器（undefined 與 null 直接變成空陣列，false 先發出棄用通知再變成空陣列，true、數字與字串依 PHP 拋出錯誤），再讀 key 並轉換，然後讀 OP_DATA 的值，最後寫入；append 在容器檢查後讀值，再插入，下一個索引已被占用時才拋出錯誤。ArrayUnset 的 Aux 為 0 或 KeyLocation，對應 ZEND_UNSET_DIM：容器是未定義變數時先發出它的警告，再讀 key（有 KeyLocation 時由指令自行讀取，未定義時發出警告並視為 null，unset 把它當成空字串 key 且不發出 null key 的棄用通知），最後依容器類型處理：陣列刪除元素，null 不做事，false 發出棄用通知但不轉成陣列，字串拋出 `Cannot unset string offsets`，其他純量拋出 `Cannot unset offset in a non-array variable`。

Location 操作數在格式 1.2 可以是變數，也可以是 FetchDim 取得的元素位置，Load、Store、IncDec、Isset、Unset、ArraySet、ArrayAppend、ArrayUnset 與 FetchDim 對兩者一視同仁。未定義變數的警告只對變數發出。

ArraySet 在格式 1.2 可以指名一個結果暫存器，寫入這個指定運算式的值。容器是陣列時結果就是寫入的值。容器是字串時這條指令就是字串位移寫入（zend_assign_to_string_offset），結果是實際寫入的那一個 byte，位移非法而沒有寫入時是 null，所以結果不能由值推得。字串容器的順序是：讀 key（KeyLocation 時發出未定義變數警告），轉換位移（null、bool、浮點發出 `String offset cast occurred`，數字開頭的字串發出 `Illegal string offset "…"`，其他字串與陣列拋出 `Cannot access offset of type T on string`），位移小於負的字串長度時發出 `Illegal string offset N` 並結束，接著才讀值（ValueLocation 時發出未定義變數警告），把值轉成字串，空字串拋出 `Cannot assign an empty string to a string offset`，超過一個 byte 發出 `Only the first byte will be assigned to the string offset`，位移超出字串尾端時以空白補齊。ArrayGet 讀字串位移時回傳一個 byte 的字串，超出範圍時發出 `Uninitialized string offset N` 並回傳空字串，安靜讀取則回傳 null。

FetchDim 對應 ZEND_FETCH_DIM_W、ZEND_FETCH_DIM_RW 與 ZEND_FETCH_DIM_UNSET，以及 ZEND_ASSIGN_DIM_OP 取得元素的部分。它讓 A 這個 Location 暫存器指向「容器 B 裡 key 為 C 的元素」，巢狀寫入因此不需要隱藏的區域變數。格式 1.4 的 FetchContainerValue 讓 B 是一個存放參照的 Value 暫存器（MakeRef 或傳參照 IterNext 的結果），對應解構時 ZEND_FETCH_LIST_W 對 VAR 容器的寫入取元素；暫存器裡不是參照時執行期以 Unsupported 結束，PHP 對這種情況的 notice 沒有建模，因為編譯器不會產生它。A 可以和 B 相同，一串巢狀層級共用同一個暫存器。執行順序是：檢查容器，讀 key，轉換 key，找到或建立元素。

| 容器 | FetchWrite | FetchReadWrite | FetchUnset |
|---|---|---|---|
| 陣列 | 元素不存在就建立為 null | 先發出 `Undefined array key` 警告再建立為 null | 不建立，元素不存在時 A 指向一個不屬於任何陣列的 null |
| 未定義變數 | 變成空陣列，不發警告 | 發出未定義變數警告後變成空陣列 | 發出未定義變數警告，A 指向不屬於任何陣列的 null |
| null | 變成空陣列 | 變成空陣列 | A 指向不屬於任何陣列的 null |
| false | 發出棄用通知後變成空陣列 | 同左 | 發出棄用通知，不轉換 |
| true、整數、浮點 | 拋出 `Cannot use a scalar value as an array`，不讀 key | 同左，但用途是 FetchForAssignOp 且有 FetchKeyLocation 時先讀 key 變數並發出它的未定義警告 | 拋出 `Cannot unset offset in a non-array variable` |
| 字串 | 一律拋出錯誤，見下 | 同左 | 同左 |

FetchAppend 表示 `$a[]`，以下一個索引建立 null 元素，不發出未定義 key 的警告，索引被占用時拋出 `Cannot add element to the array as the next element is already occupied`。FetchUnset 不能和 FetchAppend 或用途同時出現。字串容器有 FetchAppend 時拋出 `[] operator not supported for strings`，否則先照字串位移規則檢查 key（FetchUnset 不發出數字開頭字串的警告，非法型別拋出 `Cannot unset string offsets`），再依用途拋出 `Cannot use string offset as an array`、`Cannot increment/decrement string offsets`（FetchForIncDec）或 `Cannot use assign-op operators with string offsets`（FetchForAssignOp）。

元素位置是指向陣列元素的指標，和 PHP 的 INDIRECT 相同，只在取得後立刻使用才有 PHP 的值語意，所以驗證器限制它的用法。只要有一條 FetchDim 以某個暫存器為 A，這個暫存器就是元素暫存器，只能出現在「從變數取元素的 FetchDim」之後緊接的一段直線指令裡。這段指令依序只能是：

- 以自己為容器、寫回自己的 FetchDim，也就是下一層；
- Const，以及讀取「不是這串元素起點變數」的 Load，用來在各層之間讀 key 與值；
- 經由這個暫存器的 IncDec、Store、ArraySet、ArrayAppend 或 ArrayUnset 其中一條，這段到此結束。ArraySet 與 ArrayAppend 以 ValueLocation 自行讀取的值也必須是起點以外的變數，否則陣列會被存進自己的元素；
- 或是複合指定：Load 這個暫存器，接著只有 Const 與 Binary，最後 Store 回這個暫存器，這段到此結束。

跳躍目標、例外處理目標或其他任何指令都會結束這段，之後再用這個暫存器就是 `ErrInvalid`，錯誤位置是那條指令。元素暫存器也不能出現在這段以外的地方，包含 Local、Move、Drop、Push、Call、Isset、Unset、迭代器指令，或當作 key、值的 Location。是不是起點變數，以 Location 綁定的 local 判斷：被 Local 綁到兩個不同 local，或由 Local 以外的指令定義的 Location，一律視為可能是起點。這條規則逐條掃描所有指令，不可達的程式碼也一樣，所以不合規則的映像不會到達 VM。編譯器產生的序列都符合。被拒絕的寫法列在 `internal/bytecode/elements_test.go` 與 `internal/runtime/vm_stage2_elements_test.go`：在 `$b = $a` 或 foreach 之後經由先前的元素寫入、Unset 或 Isset 元素、把元素搬到別的暫存器、跳進一段的中間，以及把起點變數寫進自己的元素。目前 Load、Const 與 Binary 不會執行使用者程式碼；之後加入錯誤處理函式或物件時，這些指令可能改動同一個陣列，到時必須重新檢查這條規則。frame 結束時元素位置一律清空。

IterInit、IterNext 與 IterKey 對應 by-value 的 foreach（ZEND_FE_RESET_R、ZEND_FE_FETCH_R）。IterInit 把 B 的陣列連同「下一個位置」放進迭代器暫存器 A，並持有這個陣列。它不複製陣列：之後對原變數的寫入會先和迭代器持有的陣列分離，所以迴圈看到的是開始時的陣列。B 不是陣列時發出 `foreach() argument must be of type array|object, T given`，清空 A 並跳到 C。IterNext 取出下一個元素：寫進 Value 暫存器 B，或在 IterLocation 時指定給 Location B（和 FE_FETCH_R 直接指定 CV 相同），沒有元素時跳到 C 且不寫入 B。IterKey 把 IterNext 剛取出的元素的 key 寫進 A。迭代器暫存器只應由這三條指令與 Drop 使用。離開迴圈（包含 `break`、`continue` 到外層）時以 Drop 釋放迭代器。IterNext 與 IterKey 用在不是迭代器的值上時，執行期以 Unsupported 結束，不是內部錯誤。迴圈的回邊照樣檢查取消，IterNext 跳過大量已刪除的元素時每 65,536 個檢查一次。

ParamDefault 對應 ZEND_RECV_INIT。A 是參數暫存器。這次呼叫有傳第 A 個引數時跳到 B，否則順序執行預設值的程式碼，由它把值放進 A。編譯器的函式開頭是：每個 local 一條 Local，接著每個參數依序是 `Store Lk, r_i`，有預設值的參數前面多 `ParamDefault r_i, <那條 Store>` 與計算預設值的指令，這些指令都帶該參數的行號。函式的必要引數個數是「第一個被 ParamDefault 指名的參數」之前的參數數目。引數不足時在被呼叫函式的 frame 內拋出 `ArgumentCountError`，位置是接收第一個缺少參數的那條 Store，文字在有選擇性參數時用 `at least`，否則用 `exactly`。Call 仍然只接受個數恰好等於 Params 的引數，引數個數不同的呼叫由 CallValue 綁定。

ParamDefault 也實作 RECV_INIT 的執行期快取。B 指向的 Store 若正是接收 A 的那條，執行時在這次呼叫沒傳引數的情況下，會把剛算出的預設值記在這次執行的快取裡，但只記 PHP 也會留下的值：null、bool、整數、浮點、空陣列，以及最多一個 byte 的字串（PHP 只留下沒有 refcount 的值）。同一次執行之後再省略這個引數時，ParamDefault 直接把快取的值放進 A 並跳到 B，預設值的程式碼不再執行，所以它的警告只出現在第一次。快取屬於單次執行，不放在共用的 Program 裡。字串是否被 PHP intern 無法由值判斷的預設值，編譯器在 Compile 時以 `parameter default whose string value PHP may intern` 拒絕，包含三種：由浮點或陣列轉成的字串；PHP 沒有 intern 的一個 byte 以內的 literal，也就是原始寫法超過一個 byte（含跳脫字元）的字串、非空的 nowdoc，以及由它們摺疊出來的值（HIR 的 `Literal.Uninterned`）；兩邊都可能是字串的 `|`、`&`、`^`，以及運算元可能是字串的 `~`，因為 PHP 只對兩個單一 byte 運算元的結果 intern。至少一邊確定不是字串時，結果是整數，照常接受。

ArrayAdd 對應 ZEND_INIT_ARRAY／ZEND_ADD_ARRAY_ELEMENT，把一個陣列 literal 元素就地加入 A 暫存器裡的陣列。A 會被讀取且保持已定義，指令不重新定義它。沒有 AddKey 時 C 必須為零，值以下一個索引附加；有 AddKey 時 C 是 key，key 依 PHP 陣列 key 規則轉換（null 為空字串並發出 PHP 8.5 的棄用通知，浮點、bool 與數字字串轉成整數 key，陣列與物件拋出 `Cannot access offset of type T on array`）。AddKeyLocation 只能和 AddKey 一起出現，表示 C 是由指令自行讀取的 Location，未定義時只發出未定義變數警告並以空字串為 key，不發出 null key 的棄用通知。A 不是陣列的映像通過驗證，執行時以 Unsupported 結束，因為編譯器只會對 literal 自己的 NewArray 使用 ArrayAdd。

Unset 不會讓暫存器變回未定義，Location 仍可繼續使用，只有對應的 PHP 變數變成 unset。Exit 和 Return 一樣是終止指令，沒有後繼指令，可以是函式最後一條指令。這些指令是後端要實作的 PHP 操作，不會在載入時執行。

Push 將帶有 Value／Location 類別及可選名稱的引數加入請求內 stack。Call 只接受數量恰好等於 Params 的位置 Value 引數，CallValue 交由 Runtime 綁定可帶 Location 或名稱的引數。HostCall 只接受 Value 引數，可帶名稱，實際名稱綁定依宿主呼叫契約處理。三種呼叫指令均消耗 stack 尾端 C 筆，保留外層呼叫已累積的引數。分支合流時 stack 形狀必須相同。Return、ReturnRef、Throw、Leave、EndFinally 不得留下待處理引數。Exit 例外，因為 PHP 可以在計算呼叫引數途中結束執行。

## 例外區間與暫存器資料流

Handler 的保護區間為 `[Start,End)`，處理區塊為 `[Target,BodyEnd)`，兩者不可重疊。保護區間依 Start 遞增、同起點 End 遞減排列，只能巢狀或分離。每一對 Handler 必須符合其中一種關係：四個區間完全分離，或內層的保護區間及處理區塊一起包含於外層的單一保護區間或單一處理區塊。這允許 finally 裡的 try/catch，並拒絕跨區拼接。函式入口不得位於處理區塊。正常跳躍或順序執行不得跨入處理區塊，離開處理區塊須使用 Leave 或 completion 指令。

從區間外進入保護區間時，目標必須是 Start，Leave 也受此限制。離開 Finally 保護區間須使用 Leave 或 completion 指令，不能以一般跳躍或順序執行跨出。

IterInit 與 IterNext 的兩條出邊定義的暫存器不同：IterInit 的迭代器只在順序執行的邊上有定義，在跳躍的邊上被清除；IterNext 的元素暫存器只在順序執行的邊上被寫入，在跳躍的邊上維持原狀。IterInit、IterNext 與 ParamDefault 的出邊和 Branch 一樣受例外區間規則檢查。

Catch 入口只保證 exception register 已定義。Finally 入口不保證任何暫存器已定義。處理區塊需自行取得 local 或建立暫存值。例外不能沿用一條失敗指令本來要寫入的目的值。

Leave 明示需要展開 finally 的控制轉移。其 continuation 必須重新定義暫存器或載入 local，驗證器不假設暫存值跨越 finally 後仍可讀取。Return、ReturnRef 與 Throw 交由 Runtime 的 completion 與 unwind 機制處理。EndFinally 所在位置最內層的處理區塊必須是 Finally，繼續當次 Execution 保存的 completion。

取消、配額耗盡、輸出錯誤及引擎內部錯誤屬於宿主停止，不得被 PHP Catch 吞掉。保留回傳值、執行 finally、釋放 locals 及解構函式的順序依 `internal/runtime` 的生命週期契約，不能由 bytecode 自行指定任意記憶體清理。

## 後端必須履行指令安全契約

`Opcode.Safety()` 由程式碼決定。輸入檔案無法宣稱指令沒有副作用，或自行移除能力檢查。每個語意步驟先檢查取消、計量 Steps，再執行副作用。配置前計量實際 bytes，進入呼叫前計量 frame 與深度，長操作在內部持續檢查取消。

能力集合用來拒絕宿主不支援的指令。實際檔案路徑、shell、動態程式及 callback 仍須經當次 Policy 檢查。共享 Program 的能力集合不是執行許可，也不能取代 `internal/policy`。

VM 與原生後端必須使用同一套契約。合併指令時保留各語意步驟的取消、配額、來源位置及例外順序。純 Go 格式與驗證器不能保證後端已正確執行檢查，這部分需要 W-17 的執行測試。

## 驗證失敗與資源上限

驗證上限獨立於執行期配額，而且每項必須大於零。入口限制總 bytes、保守計算的記憶體配置量、驗證工作量、函式、指令、常數、暫存器、local、引數 stack、來源與例外表。工作量包含解析、掃描控制流程及資料流狀態，不能只限制最終結果大小。`context.Context` 可中止驗證。記憶體按實際需要的資料流狀態計量，不預留「指令數乘最大 stack」的完整矩陣。索引分段採剩餘長度計算，避免接近 int 上限時加法溢位。

| 輸入或失敗 | 結果 |
|---|---|
| 合法輸入 | 完整驗證後回傳 Program |
| 缺少身分、非法零值、空函式或未初始化暫存器 | `ErrInvalid` 或 `ErrProfile`，無 Program |
| 長度、偏移、索引、跳躍、frame 或型別錯誤 | `ErrInvalid`，附有界的位置診斷 |
| 超出驗證預算 | `ErrLimit`，無 Program |
| 版本不支援 | `ErrVersion`，無 Program |
| 未知 features | `ErrVersion`，無 Program |
| 內嵌摘要不符、可信摘要缺失或不符 | 內嵌摘要不符為 `ErrIntegrity`，可信摘要缺失或不符為 `ErrTrust` |
| 宿主能力不足 | `ErrCapability`，無 Program |
| 取消或期限到達 | context 的錯誤，無 Program |

`DefaultLimits` 為 64 MiB bytes、192 MiB 記憶體、128 Mi 工作量、4,096 函式、1,048,576 指令、262,144 常數、4,096 暫存器、65,536 local、4,096 引數 stack、65,536 例外表與 65,536 來源。這組預設值能以 Build 與 Load 接受達到指令上限、64 個暫存器的直線函式，實測載入約需 1.07 億工作單位與 145 MiB 記憶體。較寬的 frame、很深的引數 stack 與大量例外表共用同一份工作量與記憶體預算，同時接近多項上限的映像仍可能回傳 `ErrLimit`。

例外表的巢狀檢查、每條邊的保護範圍檢查及 `OpEndFinally` 都不逐一掃描例外表。驗證器先以排序掃描確認所有保護範圍與 handler 本體形成巢狀或互斥的區間族，再為每個 pc 建立最內層範圍索引，總成本為 O(指令 + 例外表 log 例外表)。驗證器依表格順序找出第一個自身檢查失敗的 handler，只對它之前的 handler 做巢狀檢查，因此同時有交叉與其他錯誤時，回報的錯誤和原本逐一比對時相同。差分測試在 20 萬組隨機例外表上確認接受與拒絕結果、錯誤訊息及最內層 finally 都與原本的兩兩比對一致，另有 20 萬組混入不合法範圍、種類與順序的例外表比對整個驗證結果。這些檢查預留的記憶體涵蓋實際配置，包含每個 pc 的四個索引陣列與每個區間的排序暫存。同一條邊有多個違規時，診斷依本體、離開本體、進入保護範圍、離開 finally 的固定順序選第一個。

`Encode` 保留 Build／Load 提供的上限，每次呼叫重新計量輸出配置、編碼與摘要工作量。Build 成功不表示 Encode 的當次預算必定足夠，Encode 可回傳 `ErrLimit`，且不回傳部分 bytes。Program 已經驗證且不能修改，因此 Encode 不重做控制流程驗證。

無限迴圈是合法控制流程。驗證器的資料流計算必須在預算內收斂，執行中的迴圈則由後端逐步檢查取消及配額。

未提供來源 bytes 時只能驗證 source map 宣告的範圍，不能證明宣告行號與實際檔案相符。函式遞迴及動態 callback 的總深度由執行期限制，驗證器只保證單一 frame 與靜態引數 stack 合法。

## 選擇較少且較容易驗證的結構

採用固定順序的長度前綴區段，省去任意 section offset、重疊與對齊規則。使用純量常數，省去循環物件還原。安全要求由 opcode 推導，省去另一套可被偽造的 effects 宣告。

磁碟編碼不綁定 VM 分派方式。分派候選包括 switch、函式表、預解碼 closure、有界 trampoline 及保留兩次檢查點的合併指令。編碼候選包括固定 16 bytes、受 16-bit 操作數限制的緊密編碼及有界變長編碼。選擇須依相同語意工作量與獨立 PGO 驗證結果，不能以合成分派基準宣稱 PHP 或應用效能勝利。

## 分派與 PGO 實測

環境：Go 1.26.5、darwin/arm64、Apple M3。每個數字是五次 150 ms 樣本的中位數，單位為 ns／完整合成工作量，越小越快。原始樣本、最小／最大值及配置量保存在 [bench-summary.json](/engineering/implementation/evidence/W-16-bytecode/bench-summary.json)。測量含每個語意步驟的取消與配額檢查，各候選的結果、步數、binary output 及中斷位置均先對照獨立控制案例。

PGO 訓練只含 arithmetic、branch-heavy、binary-mixed。heldout-alternating 未參與訓練，包含交替 taken／not-taken 分支。所有候選在獨立迴圈分派，trampoline 不使用 Go 遞迴。沒有提供生產用 default.pgo。

| 工作量 | switch | 函式表 | 預解碼 closure | trampoline | add-pair 合併指令 |
|---|---:|---:|---:|---:|---:|
| arithmetic（無 PGO） | 815.6 | 705.1 | 780.4 | 731.5 | 609.1 |
| branch-heavy（無 PGO） | 934.4 | 909.8 | 768.4 | 1062.0 | 1011.0 |
| binary-mixed（無 PGO） | 416.5 | 412.8 | 356.9 | 407.3 | 415.6 |
| heldout-alternating（無 PGO） | 479.9 | 482.8 | 410.3 | 557.1 | 515.0 |
| arithmetic（PGO） | 477.3 | 719.9 | 538.7 | 612.9 | 429.4 |
| branch-heavy（PGO） | 896.6 | 1155.0 | 728.9 | 1056.0 | 908.0 |
| binary-mixed（PGO） | 322.3 | 378.6 | 358.2 | 357.9 | 315.5 |
| heldout-alternating（PGO） | 408.4 | 517.8 | 408.4 | 523.3 | 420.8 |

W-17 採 switch 作第一個可比較的通用執行路徑。這是依準備成本及這組測量作出的起點選擇，不是所有工作量的最快者。PGO 的 switch 在保留測項與 closure 同為 408.4 ns，但 branch-heavy 的 closure 較快，add-pair 在 arithmetic 較快。函式表及 trampoline 沒有在 PGO 下普遍勝過 switch，因此目前不增加另一套正式分派路徑。

執行階段各候選均為 0 B/op、0 allocs/op。以下準備成本採 PGO 中位數：binary-mixed 的 closure 準備時間為 1383 ns、3968 B、97 次配置，add-pair index 為 95.88 ns、112 B、1 次配置，compact decode 加上新指令陣列為 243.9 ns、1792 B、1 次配置。`instruction-B` 只計原始指令陣列，`fusion-index-B` 另計合併標記，closure 儲存成本由準備基準列出，不能把 instruction-B 當成整個 Program 的常駐記憶體。

合併指令每個子操作都保留檢查點與原始 PC。是否採用 closure 或合併指令，須由 W-17 在包含真實值、Runtime 呼叫與例外的工作量上，連同準備時間及 GC 成本重測。這次只測合成操作，沒有把其他方案不能執行的案例計為效能勝利，也不以平均值掩蓋單格退步。

## 編碼實測與選擇

binary-mixed 共 97 條指令。下表使用無 PGO 中位數，解碼重用目的陣列。這是編碼成本比較，尚未加入完整驗證器成本。

| 編碼 | bytes | encode ns | decode ns |
|---|---:|---:|---:|
| 固定 16 bytes | 1552 | 263.1 | 172.9 |
| 緊密 8 bytes | 776 | 221.9 | 149.6 |
| uvarint | 485 | 449.0 | 994.6 |

v1 採固定 16 bytes，保留完整 u32 操作數與指令索引。8-byte 候選只容納 u16 操作數，無法表示目前允許的超過 65,535 常數或跳躍目標。加入 escape／wide 編碼會增加格式及驗證路徑，這次沒有其實測收益。uvarint 減少檔案 bytes，但這組解碼較慢。固定寬度並非最小或最快的磁碟格式，其優點是索引運算與驗證規則單一，且與記憶體中的無指標 Instruction 相同。

重現命令（從 repo root 執行，產物放於暫存目錄）：

```sh
BENCH_DIR=$(mktemp -d)
go test ./internal/bytecode -run '^$' -bench 'Benchmark(Dispatch|Encoding|Preparation)$' -benchtime=150ms -count=5 -pgo=off
go test ./internal/bytecode -run '^$' -bench '^BenchmarkDispatch$/^(arithmetic|branch-heavy|binary-mixed)$' -benchtime=500ms -count=1 -pgo=off -cpuprofile "$BENCH_DIR/training.pprof" -o "$BENCH_DIR/training.test"
go test ./internal/bytecode -run '^$' -bench 'Benchmark(Dispatch|Encoding|Preparation)$' -benchtime=150ms -count=5 -pgo="$BENCH_DIR/training.pprof"
```
