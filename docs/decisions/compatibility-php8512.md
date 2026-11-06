# PHP 8.5.12 已實作模組的相容性修正

本紀錄涵蓋 W-09 語法診斷、W-10 數值運算，以及 W-11／W-13 的編譯期宣告檢查。目標是修正既有 Go 實作的可觀察差異。完整 VM、其他 PHP 版本與擴充函式登錄由原工作包繼續負責。

需求：REQ-010、REQ-011、REQ-012、REQ-015、REQ-018、REQ-024。局部證據對應 T-019～T-024、T-029～T-030、T-035～T-036、T-047～T-048。產品驗收狀態維持 `not_run`。

## 語法錯誤使用唯讀的版本資料

PHP 程式由既有 Go parser 建立 AST。`diagnostics.go` 使用 PHP 8.5.12 的語法轉移資料重建錯誤訊息，包括預期 token、括號、修飾詞，以及掃描器錯誤和語法錯誤的先後順序。所有游標與堆疊都是本次解析的區域資料，語法表透過 `go:embed` 保存為唯讀字串。

PHP 的 10,000 格語法狀態堆疊與 Go 遞迴深度分開計量。可能填滿 PHP 堆疊的輸入在建立 AST 前先走有界語法檢查，未達該條件的短輸入只在解析失敗時重建診斷。每次轉移最多增加一格，檢查條件由 `128 × (token 數 + 1) + 1 >= 10,000` 推得。這會增加較大輸入的解析成本，但能保留超限與後續宣告錯誤的先後順序。重播未完成時回報宿主資源錯誤，不能當成 PHP 接受。Go 的獨立防護上限為 40,000 次受計量的遞迴呼叫。

32／64 位元參考均確認括號 9,995 層、一元運算 9,996 層、變數變數 9,997 層、巢狀 if 2,499 層、巢狀 closure 768 層及 if／while 混合 833 層的接受上限。回歸測試檢查臨界前後與錯誤優先順序，成功案例必須實際產出 AST。

`declare(encoding=...)` 依 PHP 建構語法樹時的規則辨識字面值，接受括號、純 heredoc／nowdoc 及字面值串接。一般常數計算不適用於這項判斷。兩個解析入口共用迭代走訪，長串接不會增加 Go 呼叫深度。沒有 encoding 名稱的宣告省去診斷用 AST 的重建。

`tools/php_diagnostics.py` 從固定雜湊的 PHP 發行檔產生稀疏轉移表。資料來源、授權與修改記錄在 [UPSTREAM.json](../../third_party/php-diagnostics/UPSTREAM.json)，可用 `--check` 確認產物一致。表格不包含 C parser 骨架、Zend AST、配置器或執行器。

heredoc 縮排錯誤另保留 `CallerLocation`，區分 PHP 顯示呼叫端位置與原始碼位置的情形。參考腳本從兩個不同呼叫行執行同一來源，避免把腳本的固定呼叫行誤當來源行。Go SDK 的 `SourceLocation` 繼續指向傳入程式的檔名與行號。

## 編譯常數共用運算，保留各入口的計算順序

`internal/phpnum` 提供不持有請求狀態的數字字串解析、比較、格式化與冪次運算。runtime 的原入口保留，IR 改用相同的純函式，避免兩套近似規則產生不同結果。

PHP 對一般運算式與陣列、宣告初值的編譯順序不同。`constant_syntax.go` 在需要的入口先分析 AST，記錄已知值與選中的分支。`constant_compile.go` 處理已降低的 HIR。兩者共用運算規則與單一運算式的資源預算，未知值保留到執行期。static 初值也先做這項分析，動態函式呼叫仍保留為 HIR。

常數陣列使用有序 entry slice 與 key 索引，合併、比較與展開都建立本次編譯擁有的值。`iter.Seq` 讓 AST 與 HIR 共用陣列組裝程序，不另外建立暫存元素 slice。完成的已知與未知結果都記錄在區域 memo，重用時仍計入工作量及深度。

每個運算式最多使用 1,048,576 單位工作量、32 MiB 新增資料、65,536 個已訪問節點及 2,048 層計算深度。超限回報宿主資源錯誤，不能改成「未知常數」後繼續接受。沒有被短路規則選到的大子樹只影響容量估算，不會自行觸發計算失敗。長 unary chain 使用顯式堆疊處理，沒有放寬總量限制。

NaN 轉成字串時，PHP 8.5.12 會發出 `unexpected NAN value was coerced to string`。這個警告在實際轉換的位置發出。一般運算式的字串串接，會在編譯時把每個常數運算元轉成字串。常數運算式則只在整個串接都能算出時才轉換。memo 讀取值時不發警告，同一個運算元因此只警告一次。

執行期程式碼的這類檢查只判斷運算元是不是常數，不會讓編譯失敗。串接與插值的結果一定是字串，不必計算，所以很長的字串串接不會碰到常數計算上限。超過自身預算的運算元當成執行期才計算，PHP 對它的轉換那時仍會發生。常數運算式與 `::class` 仍要求完整計算，超限時回報宿主資源錯誤。

屬性名稱與靜態屬性名稱若能在編譯時算出陣列或 NaN，Phego 會像 PHP 一樣當場轉成 `Array` 或 `NAN` 並發出警告。必須在執行時才能建立的陣列不會折疊，例如 64 位元的 `[9223372036854775807 => 1, 2]` 會在執行時拋出錯誤，`[1.5 => 1]` 與 `[null => 1]` 會在執行時發出 Deprecated。`??=` 會把左邊編譯兩次，所以左邊每個屬性名稱的轉換警告也會出現兩次。可變變數的名稱（`${[1]}`）也一樣轉換。方法名稱與類別運算元用同一套編譯期計算，算出非字串常數時分別回報 `Method name must be a string` 與 `Illegal class name`，行號是名稱運算式最後編譯的那一行。`??=` 第二次的轉換警告沿用第一次的行號。

括號會讓類別位置的名稱變成運算式。`(A)::f()`、`new (A)`、`$x instanceof (A)` 與 `(A)::class` 讀取常數 `A` 的值，不是類別 `A`。剖析時已是字串的類別運算元是完整限定名稱：`('A')::class` 在任何 namespace 都是 `"A"`，`('self')::f()` 回報 `'\self' is an invalid class name`。常數運算式裡的 `(A)::C` 因此回報 `Dynamic class names are not allowed in compile-time class constant references`，`(A)::class` 回報 `(expression)::class cannot be used in constant expressions`。加了括號的字串字面值，以及剖析時已串接完成的字串，可以當類別名稱，例如 `const X = ('A')::C;`。

## 宣告檢查保留延後繫結

`class_compile.go` 檢查類別本身，`attributes_compile.go` 檢查 attribute 的參數、target、重複與內建驗證規則。`class_binding.go` 區分可判定相容、不相容與尚待載入的依賴，`class_link.go` 只對當下可繫結的宣告檢查繼承。不能因為同檔出現某個類別，就假設它已經完成宣告。

檢查包含方法與常數的可見範圍、final／static／abstract、readonly class 的隱含屬性規則，以及 property hook 的型別與缺失實作。hook backing 掃描停在巢狀函式或類別的作用域，只檢查會在原 hook 執行的子運算式。屬性名稱區分大小寫，方法名稱依 PHP 規則比較。

hook 是否使用自己的屬性，依 PHP 剖析完成時的值判斷。`$this->x`、`$this->{'x'}`、`$this->{"x"}`、`$this->{'x' . ''}`、不含插值的 heredoc，以及 `${'this'}->x` 都算 backed。`$this->{(string)'x'}`、`$this->{X}` 與 `$this->{"$x"}` 要到執行時才知道名稱，所以算 virtual。`$this->x()` 是方法呼叫，不算使用屬性，但它的物件、名稱運算式與引數仍會檢查。常數運算式裡的 closure 本體照一般程式碼降階。

決定能否提早繫結的 `bindingMethod`，也是回報方法宣告不相容的唯一依據，兩個步驟不會得出不同結論。必填參數數量算到最後一個必填參數，放在必填參數前的預設值也算必填。錯誤訊息只在最後一個必填參數之後顯示預設值。回傳型別裡的 `static` 依 `zend_type_permits_self` 判斷：父型別允許 object，或含有目前類別本身或祖先的具名型別時才相容。純交集型別看每個成員，union 裡的交集不列入。final 類別以自己的名稱取代父類別的 `static` 時，若名稱尚未載入，會延後判斷。

magic method 的回傳型別依 `zend_check_magic_method_return_type` 的型別遮罩檢查。`__isset()` 可以宣告 `true` 或 `false`，`__debugInfo()` 可以宣告 `null`，`never` 一律允許。類別型別（包括 `static`）只能用在要求 object 的 `__set_state()`。

`core_classes.go` 只提供已固定 profile 的核心類別描述。擴充函式、DateTime、PhpToken 等完整登錄仍屬 W-29。編譯語料以檔名與完整診斷列明剩餘 12 項缺口，不再按整類診斷前綴排除繼承或 hook 錯誤。

## 32 位元冪次運算的正確性與代價

32 位元 profile 改用固定精度的 `math/big.Float` 計算，保留 i386 參考環境的 53 位元中間算術、擴展指數範圍與運算順序。超越函數採固定 256 位元工作精度與 128 項級數，`sync.OnceValue` 只初始化唯讀數學常數。64 位元 profile 繼續使用已驗證的 glibc 相容純 Go 運算。

原本運算矩陣的 51 個已知差異與 30,000 筆 i386 `pow` 語料已改為精確比對。額外的 60,000 個浮點輸入與 20,000 個整數指數輸入也已比對。這些結果只證明固定參考環境與實際測試輸入相符，不能證明所有 x87 CPU 的每個輸入都相同。

這是有成本的相容性修正。Apple M3 的局部測量中，i386 分數指數路徑約 48 微秒、71 KiB、1,038 次配置，64 位元既有路徑約 6.4 奈秒且零配置。重用級數暫存值已移除不必要配置，整數指數也省去不會被使用的最後平方。這些微基準不是完整 VM 效能，也不構成超越 PHP 引擎的證據。

## 重現與限制

- `python3 tools/verify_compile_fixtures.py` 以固定的 32／64 位元 PHP 8.5.12 映像重新執行 11,334 筆編譯案例，逐項比對層級、訊息、行號與順序。工具不改寫預期結果。
- `TestLowerMatchesCompileReference` 對照 5,307 份 php-src 編譯輸入，剩餘 12 項 W-29 缺口維持明列。
- `TestCompatibility*` 與 `TestConstant*` 保護修正、短路、錯誤順序與資源限制，完整命令及結果見[執行證據](../implementation/evidence/compatibility-php8512-20261104.json)。
- PR #129 審查後新增的 572 筆編譯案例在 `TestCompatibilityReviewOracle` 比對，`php -l` 看不到的執行期名稱與類別運算元由 `TestCompatibilityReviewRuntimeOperands` 檢查，命令與結果見[審查後執行證據](../implementation/evidence/compatibility-php8512-review-20261106.json)。

下列差異已用固定映像確認，尚未實作，因此不在比對資料內：

- PHP 會在編譯時把 `NAN`、`PHP_INT_MAX` 等內建常數換成值，Phego 仍留到執行時。`[NAN . '']` 因此少了編譯期警告，`$o->{[1, ...[PHP_INT_MAX => 1]]}` 也少了 `Array to string conversion`。這需要依 profile 提供內建常數表。
- `ArrayObject` 等 SPL 類別還沒有繼承資料，繼承它們的類別不會提早繫結，相關的宣告錯誤要等 W-29。比對資料改用意義相同的使用者類別。
- 常數運算式裡，PHP 先折疊類別位置的運算式再檢查，所以 `const X = (A::class)::C;` 可以編譯。Phego 仍回報動態類別名稱錯誤。內建常數也屬於這種情況，例如 `const X = (PHP_EOL)::C;` 在 PHP 可以編譯，`(null)::C` 回報 `Illegal class name`，同樣需要內建常數表。
- attribute 引數裡的 closure 目前只檢查常數運算式，本體沒有降階，所以本體內的編譯期警告與錯誤都不會出現，例如 `#[A(static function () { return 'a' . (1e999-1e999); })]` 少了 NaN 警告。
- 方法相容性錯誤依子類別的方法順序回報，PHP 依父類別的順序，而且屬性錯誤先於方法錯誤。訊息中的出處取最早宣告的祖先，PHP 取最近覆寫的那一層。預設值預覽也還沒照 PHP 截斷長字串。
- `$a[] ??= 1;` 應回報 `Cannot use [] for reading`。多行 hook 的 `Cannot specify default value for virtual hooked property` 應指向 hook 最後一行。

所有 PHP 參考執行都在固定映像、無網路、唯讀檔案系統、限制記憶體與程序數的容器內完成。預設核心保持純 Go，公開 Go SDK 沒有改名或更換簽章。

既有不安全 PHP 參考案例保留在 W-13 的受阻記錄中。本修正沒有把崩潰、逾時或未執行的情境算成通過。下一個依賴已滿足的整合工作包為 W-17，需把編譯產物接入 VM 後再驗收完整 PHP 程式。
