# W-13 物件模型與邏輯生命週期

對應 REQ-012、REQ-018、REQ-019、REQ-020、REQ-021，局部情境為 T-023、T-024、T-035～T-042。本紀錄描述實作中的選擇，尚不構成整票或完整 Language profile 驗收。

## 屬性的位置與成本

類別描述保存不可變的屬性名稱、宣告者、存取權、型別、hook 與位置索引。每個物件一次配置固定長度的宣告屬性值陣列，配置後不再 append 或更換其 backing array。父子類別同名 private 屬性以各自的宣告者區分。

動態屬性保留插入順序與名稱索引，值的位置使用不搬移的鏈結分塊。小物件依序配置 1、2、4、8、16 格，其後每塊 16 格。每塊的 header 與固定長度 entries 合併在同一個普通 Go struct 配置，不另存可擴充的分塊目錄與順序 slice。名稱索引直接指向固定位置。刪除後不重用位置，重新加入同名動態屬性會排到尾端，迭代時以目前索引排除舊位置。宣告屬性重新初始化則維持其原位置。

| 做法 | 結論與依據 |
|---|---|
| 直接沿用 W-12 的 phpArray | 不採用。物件屬性不需要數字鍵正規化、packed 轉換、排序或 array cursor；原布局還有每格 bucket 配置。 |
| Go map 的值直接作為位置 | 不採用。map 值不能提供可逃逸的固定地址，也不提供 PHP 的順序。 |
| 每格獨立 `*value` | 位置穩定，但保留每格配置成本。宣告屬性可在不改語意的條件下一次配置，因此採用固定值陣列。 |
| 動態屬性分塊 | 採用。擴充名稱索引與連結新塊不會搬移既有值；小塊起步避免小物件浪費。 |
| 跨請求池化、unsafe 或 Go 實驗性 arena | 不採用。固定位置可由普通 Go slice 保證，不需要額外生命週期或不安全指標契約。 |

物件 identity 與屬性表 header 合在同一個 Go struct 配置，物件保留指向其中 header 的 interior pointer。宣告值陣列另一次配置，位置維持固定。clone 複製值到新物件既有的表，避免先建立再丟棄一份 header／defaults。resource 只配置 identity，不負擔屬性表。

Go 1.26.5、darwin/arm64、Apple M3 的量測中，value 為 64 B、bucket 96 B、object 96 B、propertyTable 64 B。32 屬性表的 W-12 陣列布局為 8880 B／47 次配置，宣告表為 2304 B／1 次，動態表為 7704 B／15 次。含物件建立、寫入與釋放的結果如下，原始 main 為 `4578556dcba800b34a9f0ad8a0cc8359e4e6e0d1`。

| 屬性數 | 原始 main 動態物件 B／配置 | W-13 動態物件 B／配置 | W-13 宣告物件 B／配置 |
|---|---:|---:|---:|
| 0 | 160／2 | 160／1 | 160／1 |
| 1 | 664／6 | 544／4 | 224／2 |
| 4 | 1000／11 | 1136／6 | 416／2 |
| 32 | 9040／49 | 7864／16 | 2464／2 |
| 256 | 72624／282 | 51224／36 | 18592／2 |

4 個動態屬性比原始 main 多 136 B，但少 5 次配置。這個固定位置與成長容量的成本明列保留，不能宣稱所有情境記憶體都下降。32 與 256 屬性的成本支持採用專用布局，沒有 PHP VM、真實應用或競品效能通過的含義。原始樣本、基準原始碼與重跑命令列在 W-13 evidence。

PHP 比較物件時，僅有宣告值的直接路徑與已建立屬性檢視的 hash 路徑，對未初始化值的結果不同。runtime 以 `materialized` 保存後者狀態，dynamic index／blocks 也會選擇 hash 路徑。建立屬性檢視只設一個 bool，動態寫入才建立 map。clone 與 lazy 快照保存此狀態，rollback 同步還原。這個 bool 讓表從 56 B 增至 64 B，但本機 Go allocator 的獨立表與合併物件配置級距均不變，省掉宣告物件屬性檢視的空 map。配置前後量測位於 evidence；此級距結論只適用實測 Go host，不由 PHP guest 的整數寬度推論。

## 參照限制與請求狀態

型別限制按實際屬性位置附加，別名寫入同一 reference 時一起驗證。unset、物件釋放、clone 與循環釋放必須維護來源的加入與解除。限制放在 heap 的按需側表，避免每個 reference 都增加一個 slice header。value 維持 64 B；為拒絕跨請求的活陣列與 reference，各自增加一個 heap owner 指標，phpArray 為 88 B、reference 為 80 B。

typed-source 的來源順序可被 `__toString` 重入觀察。驗證保存原長度，但每一步讀取目前 metadata cell。移除採 PHP 的「第一個相同 property-info、尾端補洞」順序，clone 的重複 metadata 另外調整存活 Go slot 對應。繼承複製描述時保留 declaring owner 與 storage key 作為宣告身分，先用 info 指標相等快速判斷，再比對 owner／key；重新宣告、private 同名與不同 trait 使用者維持各自來源。這不新增索引或描述欄位，避免註冊時另查 canonical descriptor 的成本。尾端只清 slot 指標，保留不可變 info 給進行中的驗證；不配置每次寫入的快照或變更帳本。來源容量為 1、4、倍增，刪除後四分之一滿且至少四格時縮半。兩來源比先前 Go cap 2 多保留 32 B，此成本由原生 append／remove 區辨要求，不能只省容量而改變 PHP callback 次數。跨原生 realloc／free 的輸入沒有驗證，Go 保留舊 storage 的安全性不構成該類輸入的相容證據。

型別來源先區分原樣接受、拒絕、可能轉型，再對同一原始 RHS 比較每次轉換。mixed 在前、int 在後而寫入 array，原生先報 inconsistent conversion；不能提早排除實際不可轉換的值。`self`／`parent` 的原樣接受保留屬性宣告類別。轉型拋出的 PHP Exception 由 TypeError 取代並保留 previous，普通無法轉換則只有一個 TypeError。

參照寫入在轉換期間持有原 reference 及 RHS。alias 被 unset 時更新原位置而不重建名稱，被重新綁定時追隨目前 reference，重新核對 request owner，依固定 PHP 不重跑新目的地的型別限制。寫入後先釋放 RHS，再釋放舊 target garbage，精確保留 `convert,convert,rhs,old`。Go 測試使用真正的可寫 cell；額外複製／持有測試 operand 會掩蓋這些生命週期錯誤。

[建構函式回傳的暫存值](../../internal/runtime/heap.go#L88)（constructor-style temporary）尚未存入 PHP 位置，持有計數為零。對尚未認領的 array／reference 圖，先驗證所有可達值，整體成功後才提交 owner。已認領的同請求值使用固定成本檢查，不重走整張圖。bind 與 foreach 綁定會先保存兩個可寫位置的原始零計數狀態，再各取得所需 owner，避免兩位置指向同一個暫存值時，先取得的計數掩蓋另一位置的需要。普通 Go value 複製本身不取得 PHP owner。

類別描述不保存物件欄位、static 值、enum 活實例、lazy 初始化錯誤或 hook 重入狀態。這些狀態由該請求的 heap／object 擁有。Go callback 是受信任的宿主程式；程式描述中的 callback 不應捕捉 PHP 請求的可變值。

resource 使用既有 identity 指標欄位與獨立的請求登錄，不在每個 PHP value 加一個 resource 指標。使用時核對 owner、種類、identity 與代次。關閉先提交 closed 狀態，再呼叫宿主清理器，防止重入重複關閉。PHP scope 釋放與 W-14 最終清理共用該登錄，實際 I/O 與能力授權仍由對應模組處理。

## 呼叫與生命週期

明確 unset 的宣告屬性以 undefined 值內原本未使用的 boolean 位元區別，維持 value 64 B，不另配置 bitmap 或側表。這個標記讓 magic get／set／isset／unset／reference 與真正從未初始化的 typed 屬性分流，clone 與 rollback 會保留標記。弱型別轉換可能執行 PHP callback，寫入前必須重新核對 receiver 仍存活並重新取得 slot。readonly 初始化資格依 PHP 在轉換前判斷；原生重入案例證明內層賦值後外層仍可完成首次賦值，因此不追加轉換後的 readonly 拒絕。

callback 回傳 reference 時，按值讀取先保護 target，再消耗暫存 wrapper，於 W-14 取得回傳 owner 前完成。按參照綁定則直接為目的地取得 owner，避免把 Go 區域複本誤算成第二個來源 owner。magic setter／unset、magic 與 hooked isset、clone 的未使用結果都在操作結束前消耗，清理例外保留 previous chain。clone 以單一 `methodHold` 保護新物件，涵蓋回傳自己、含自己的陣列與回傳值解構重入 readonly 初始化，不增加持有權帳本。

clone 先複製及分離無外部 alias 的 reference，再單次掃描 schema，僅對仍共享的 typed instance slot 登錄來源。這排除 static／virtual／untyped 宣告與同名動態位置，也移除每個屬性都重掃 schema 的平方成本。單一來源保護改用 stack `methodHold`，省掉 80 B／一次配置；256 typed 屬性的五次樣本由 105616～114549 ns 降至 19204～19659 ns，plain 256 由 18672 B／3 次降至 18592 B／2 次。來源容量的額外保留空間與 warm-up 不包含在穩態 B/op 結論內。proxy backing clone 失敗時，尚未完成的 wrapper 直接清理其 storage，不額外執行 wrapper destructor；backing clone 的真實解構與完整 previous chain 保留。

clone handler 從實際類別解析，再以呼叫端 scope 檢查可見性。檢查先於 lazy 初始化與 clone 配置，省掉拒絕操作的不必要配置與副作用。具名 magic 參數重複檢查重用已打包的陣列索引，不另建集合。union 的 int 轉換溢位時繼續嘗試允許的 string／bool，包含固定映像的 NaN 診斷。已知零個實體屬性的 lazy 類別直接建立普通物件；未知舊描述不套用這個快速路徑，initializer 的請求 owner 仍先檢查。

實例查找跳過不屬於目前類別的祖先 private 宣告，宣告者 scope 則先取得自己的 private 位置。static 查找保留原有可見性規則。PHP-facing 名稱在 instance resolver 驗證，避免 NUL 開頭的輸入命中內部 mangled key；既有錯誤分流維持 magic 優先、沒有 magic 時的 isset 回傳 false。動態 isset 先取 lazy backing storage，不能只看 wrapper 空殼。

有 `__toString` 的類別加入不可變 Stringable interface 描述，繼承與 trait 共用同一套型別匹配。enum 以四值 `enumKind` 區分普通類別、unit、int-backed、string-backed，填入 classSpec 原有 padding，實測仍為 184 B。這能表示零 case enum，也能在建置時驗證 backing 數量／型別。`name` 與 `value` 納入 readonly 宣告布局，UnitEnum／BackedEnum 描述一併加入；建立 case 時只初始化一次，不再使用無屬性限制的動態位置。

回傳型別驗證放在方法自身的 W-14 callback 內，先完成轉換，再移交回傳 owner，最後釋放局部值。需轉型的 raw 結果由 `checkMethodReturn` 暫時持有並消耗一次。普通 PHP 轉換例外在 return 邊界由 TypeError 取代並保留 previous；argument 邊界保留原例外。成功的 argument 轉換若在釋放來源時解構拋例外，仍執行方法 body 並保留 pending 事件。取消、exit 與 internal failure 不使用這條繼續執行規則。

連到 typed property 的按參照回傳，值不符合回傳型別時，在轉換前拒絕，包含 mixed／union 來源與 int→float，避免改寫來源或執行 `__toString`。合法 callable 先做請求內符號／scope 驗證，屬於原樣接受，不會被這個限制誤認為需轉型。

方法表保存建置後不可變的 descriptor 指標。新定義與 trait 套用各有自己的 descriptor，純繼承共享原 descriptor；trait 綁定只複製描述本身，已凍結的參數與 defaults 可以共享。這讓方法實作身分可直接比較，也省掉原先每次 lookup 複製 descriptor 的配置。固定本機微型測試中，單次方法呼叫從 208 B／4 次配置降至 112 B／3 次配置。再把只保存單一 receiver 的 slice 改成一個 `methodHold` 值，降到 32 B／2 次配置。這批前後樣本執行時有其他工作，不能用其時間宣稱速度提升。

最後兩次配置來自 union／intersection 拆分的暫存 `[]string`。Go 的 `iter.Seq[string]` 直接逐段產生原字串切片，保留空 token、括號深度與提前停止規則，不增加快取或描述欄位。五次前後微型量測將無參數、回傳 int 的呼叫降至 0 B／0 次配置；全套 433 案及既有 DNF 控制維持一致。scanner 狀態物件也可省配置，但呼叫端較繁，現有 iterator 已達到目標，因此未採用。量測只支持這個操作，不推論所有方法或應用零配置。

WeakMap 沿用固定 PHP 的空標準屬性比較，兩個不同實例的 entries、順序、數量與循環均不影響相等結果。不同 class 仍不可比較。以 builtin identity 在 owner、materialization 與 recursion bookkeeping 前直接回傳相等，因為這條路徑不讀值也不執行 callback。這保留既有入口驗證，不新增另一套內容比較；Codex 建議的 entries 比較已被雙寬度原生區辨否定。

backed getter 在自身 frame 呼叫 isset 時直接檢查 backing 是否 undefined／null，避免轉成未初始化讀取錯誤。magic call 打包參數時保存是否已遇到具名參數，後續位置參數立即拒絕，不執行 `__call`。輸入對應 `call_user_func_array`，原始碼的 unpacking 診斷仍由 frontend／VM 整合驗收。

virtual setter-only 的 isset 走既有 readObject 錯誤路徑，backed setter 的自身 isset 保持 false。裸物件 callable 直接讀有效類別的 `__invoke` handler，不套 caller visibility 或 `__call`；明確 method-array callable 保留 magic 分流。private／protected 宣告警告由獨立原生探針保存，完整 frontend 裸物件呼叫與宣告診斷未驗收。

scoped private 查找以一次 ASCII 正規化及既有 method map 取代整表掃描，private owner 規則不變，不增加索引或快取。小寫 public、1024 個無關方法的五次量測從約 6.3 微秒降至 19 奈秒且零配置。大寫 private 的三次 overlay 量測新增 8 B／一次配置，從約 3.1 微秒降至 32 奈秒；接受此字串成本以免線性掃描，不宣稱全面零配置。各大小、無 scope 控制及重播方法見 `W-13-r4-review.json`。

類別 destructor adapter 只捕捉 callback 本身。Go `-gcflags=all=-m=2` 證明從以 reference 捕捉整個 184 B 的 classSpec，改成以 value 捕捉 8 B 的函式值。沒有新增狀態或改變 callback 行為，原始編譯器結果保存在 `W-13-destructor-capture.txt`。

執行 scalar 或 Stringable 轉換前先保護 receiver 與全部實參，避免前一個參數的 PHP callback 釋放後續參數。綁定完成後先解除原 operand owner，已轉成 scalar 的來源可在方法 body 前解構，其他值由參數 owner 接手。參數按宣告順序釋放，包含以相反順序傳入的具名參數。呼叫端不用回傳值時，還必須保存其 operand 存活範圍，先捨棄結果，再清理呼叫端暫存值；不能把使用與不使用結果的兩種輸入混為一談。

hook、lazy initializer 與正式類別 destructor 接到 W-14 的 frame／completion 邊界，沿用取消、深度限制、局部值釋放、fatal 與例外次序。回傳值在 frame 清理前取得 owner，離開後移交暫存值，避免 callback 回傳的物件在交付前被解構。既有 prototype helper 與公開 Go API 的名稱、簽章不變。

hook bypass 只看目前 PHP frame 的 receiver 與 property prototype。heap 保存一份目前 hook 狀態，真正呼叫函式時保存、清空，返回時還原。這省去每物件的 property-hook map 與 hook stack，也能讓 getter 經 helper 再讀同一屬性時重新進入 getter。父類別非 private 屬性的 override 保留 prototype identity。proxy hook 存取自己的 backing instance 時也視為相同 receiver。一般 finally 沿用原 frame，不另外清空狀態。

不同原始類別的物件比較直接回傳不可比較，不初始化 lazy object。需要比較內容時，先持有左右 operands，清理按左、右次序進行。兩個指定映像對 lazy properties 的初始化次序不同，arm64 是左、右，i386 是右、左。`comparisonPropertiesPHP8512` 明確實作這兩個固定 profile 的結果，不能把整數寬度當成所有其他平台的行為證據。上游 [zend_object_handlers.c:2352](https://github.com/php/php-src/blob/136e049ac10b032c83018e65b63b6df858943750/Zend/zend_object_handlers.c#L2352) 在同一個 C 呼叫求值兩個 properties 引數，原生 fixture 的雙寬度結果位於 `review.php`。

類別登錄先驗證完整 parent chain，再發布到當次請求的登錄表。失敗不留下 child，重試也不能繞過 parent 同名衝突。已發布的不可變 descriptor 可以使用同指標快速返回，不需要複製整張登錄表、鎖或 rollback log。

物件編號在一般執行時使用 LIFO 空槽，等物件與其屬性、弱表、lazy storage 的重入清理全部完成才放回。循環釋放則在整批清理完成後放回。兩個 shutdown 入口在反向 global sweep 後停止重用空槽，再按 object-store 位置呼叫 destructor，期間新增物件排在高水位之後。省略這個狀態會改變 PHP 可見的解構順序。

所有後發生的 PHP 清理例外沿用 W-14 的 `mergeObjectErrors`，取代先前事件並保留 previous。magic call 不另外維護 `errors.Join` 次序。無 execution 的宿主 callback 同樣遵守 constructor-style temporary 契約，無效回傳值由既有 `discard` 清理。

resource 的明確 close 使用呼叫端 context。execution 內因最後 owner 消失而觸發的自動 close，使用該次執行已驗證的 cleanup context，維持原 deadline。沒有 `Background`、`WithoutCancel`、額外重試或新 deadline。取消中的操作可以提交釋放而仍完成一次清理，cleanup deadline 自己到期則照實回報。

GC 不依賴 Go finalizer。固定 PHP 映像的 `gc-created` 案例證明析構後須再掃描一次，新一輪碰到的 destructor 會執行，但其物件可延到下一次收集才計數釋放。巢狀 `gc_collect_cycles` 回傳零。原型單次重算原 white 集合漏掉新根，已由失敗回歸重現。

同一次 GC 內多個解構的順序不能沿用 W-06 的「依實作不同」作為本票已接受差異。已查證 PHP 8.5.12 的 root buffer 有 LIFO 空槽、由尾端補洞的壓縮、活根移除與最多一次重跑。以位置表取代單調序號與排序，也省去每輪 map→slice→sort。

64 組原始圖序列中的第 15 組曾出現兩種寬度都一致的順序差異。固定映像的唯讀診斷確認，PHP 外層 by-value foreach 的測試表在 GC 後仍是一個 refcount 2 的根。PHP 的 `zend_gc_check_root_tmpvars` 會在收集完成後重新加入活躍 loop／temporary 值，原型沒有這個步驟。W-12 的 foreach 操作現在登錄其活躍範圍，GC 在最後壓縮後按 frame／live-range 順序重新加入。圖序列重播也保存相同的外層 foreach 存活範圍，不刪除或放寬原始事件預期。

此差異的處理包含補齊 runtime 行為與輸入狀態，不把它列為允許差異。尚未證明一致的其他條件保留為未完成，不能刪除輸出或把失敗改成 skip。上游依據為固定 PHP commit `136e049ac10b032c83018e65b63b6df858943750` 的 `Zend/zend_gc.c`，尤其 root compaction 與 `zend_gc_check_root_tmpvars`。

lazy ghost 失敗時按宣告順序還原快照。固定 PHP 8.5.12 會直接覆寫 destructor 重入寫入的新值，不解除這筆持有計數。單案對照證明，提前 release 會提早執行原本未執行的 destructor，因此不採用。物件與 resource 保留在當次請求的登錄，由既有 shutdown 與資源清理處理。額外的 orphan ledger 會增加保存空間，而且不能新增 GC edges 或 roots，否則又改變可觀察行為，目前沒有採用的必要。

`diagnostics/lazy_rollback_pair.php` 將 declared 與 dynamic rollback 連續執行，固定 arm64 PHP 發生 `zend_mm_heap corrupted` 並以 133 結束，i386 也發生記憶體損毀並在本機模擬環境中逾時。兩種單案各自執行及追加 GC 都以 0 結束，Go 也符合單案檢查點結果。使用者於 2026-11-02 另外明確核准保留單案精確比對與原始崩潰證據，連續案例明列相容性受阻、不計通過，其他驗收與審查完成後允許合併。此項核准與下列弱參照復活例外各自限於指定輸入。原始結果位於 `W-13-lazy-rollback-diagnostic.json`。

## R6：拒絕順序、參照參數與呼叫類別

unset 先檢查不需讀取值的 hook 與 setter 權限，並依 readonly slot 的實際狀態及 clone reinitialization 判斷。已明確 unset 的位置先嘗試 magic，遞迴 guard 仍保留；缺少的動態屬性也先走 magic。需要初始化時沿用 `objectForProperty`，初始化或選到 proxy backing 後重跑 descriptor 檢查，避免 ghost 原地寫入 readonly slot 後被錯誤移除。receiver 未變時沿用已取得的 slot，省掉第二次查找。

typed property reference 傳入 by-reference 參數時只接受目前型別，不能先執行 scalar／Stringable 轉換。request-aware callable 驗證仍屬於合法接受，普通未受型別來源約束的參照維持 weak coercion 與 writeback。只有 typed source 分支額外查 callable，普通參照由原轉換路徑檢查一次。variadic 共用 binder 與錯誤建構，只保存已處理的位置參數數量；具名參數不推進它，兩種 variadic 錯誤均省略參數名稱。

非對稱屬性的權限檢查採 lexical scope，PHP 訊息則採 called scope。直接從目標物件推導會在操作其他物件時出錯，僅使用方法宣告者也會漏掉繼承呼叫。因此新增當次 heap 的 called-scope 指標，callback 入口以 `defer` 保存及還原，execution 版本涵蓋 local cleanup。方法、static、hook 與 destructor 傳實際 called class；一般 callback 清成 nil。既有入口名稱與簽章保留為 wrapper。這增加一個 request-local 指標及進出 frame 的操作，沒有每次呼叫配置；微型時間樣本較慢，明列成本。現有無 class lazy host callback 仍傳 nil，帶 class scope 的 Closure 由未完成的公開介面整合。

明確 resource close 不先檢查已取消的 operation context。它保留 heap、owner、generation、kind、open 檢查，並把原樣傳入的 cleanup context 交給一次性 close callback。普通操作繼續檢查 operation cancellation。兩者共用 `resourceRecord` 的 owner／generation 查找，保留簡短 state 檢查，避免為抽共用層增加普通操作呼叫；固定訊息使用 `errors.New`，不呼叫格式化器。完整雙寬度對照、失敗重播、資源控制與前後基準見 `W-13-r6-review.json`。

### raw-unset、proxy magic 與 guard 的存活範圍

raw 設值後再 unset 的位置已有明確狀態。固定 PHP 8.5.12 的讀取、isset 與取得參照都不執行 initializer，但沒有 `__set` 的寫入仍會初始化。因此不能一律繞過初始化；讀取與寫入透過兩個可 inline 的 wrapper 傳入共用 resolver，保留 target 與 running 狀態的原優先順序。magic setter 先於初始化，包含拋例外與 guarded reentry。

已初始化 proxy 的動態位置仍先走 wrapper magic。遞迴進入 backing 時，guard 是否轉移取決於操作及是否真的發生初始化；把所有操作一律複製 guard 會少掉 PHP 應執行的第二次 callback。既有 target guard 由原呼叫者擁有，內層不得清除。原生區辨案例包含 declared／dynamic、直接／經其他屬性初始化、不同屬性及既有 guard，全部保留精確事件。

forwarded guard 只用 stack 值保存本次取得的 target 與 key，還原時先清空所有權再刪除，因此可安全地提早還原並保留錯誤路徑的 defer。取得參照的 guard 在來源位置解析後、綁定目的地前就必須結束，否則目的地解構中的 getter 會被錯誤抑制。`isset` 遇到不存在的 backing 直接返回 false；GET 重用查得的 slot，已有值時不安裝不可觀察的 guard。unset 的 defer 移至單次轉交 helper，避免放在遍歷迴圈中。這些調整不新增物件狀態，magic key／map 仍可能配置。

曾考慮加入 proxy cache-link flag，但固定來源的 initialized proxy 使用 `properties=NULL` 與 `UNINIT|LAZY` slots，既有狀態足以區分，沒有新增旗標的必要。共享 key 建構與 read/write wrapper 均由 Go 編譯器確認 inline；不是應用效能證據。兩種寬度各 587 案、原 564 案不變，修正前失敗、編譯器輸出及限制見 `W-13-r6-followup.json`。

## R7：raw 位置、旗標與完成計數

Ghost 初始化只安裝仍 lazy 的宣告預設值，保留 raw 值、明確 unset、reference identity 與物件持有權。一般新物件仍安裝全部預設值。已有值不等於 non-lazy：proxy factory 普通寫入後失敗時，固定 PHP 會保留值及 lazy 旗標，因此不能只看 `value.kind`。

狀態放在每個 lazy object 的稀疏 `nonLazy` map，key 是實際 property storage key。raw adapter 先解析有效 descriptor，再以同一個 key 判斷、標記與寫入。父類別 private 與子類別同名屬性不能按名稱第一次命中判斷，否則會重複扣減剩餘計數。`isLazyPropertyInfo` 同時服務已解析 descriptor 與 by-name 查詢，後者先選有效宣告。一般 `h.assign` 不能猜測呼叫者是不是 Reflection。

不把旗標放進 value padding，因為值會隨賦值、回傳、clone 與參照綁定搬移，位置狀態不應跟著移動。維持這個區別會迫使所有值複製路徑多負擔協定，成本較高。位元集合需增加索引及溢位處理，目前沒有原生工作負載證明其收益，保留為未量測選項。稀疏 map 只有明確 raw 操作才配置，普通物件的 cell 與 property table 不變。

raw 寫入在 coercion 前標記，讓 callback 看見正確狀態。錯誤僅在原位置 lazy、值仍 undefined 且物件仍 pending 時回復旗標。實體寫入共用 `writeObjectSlot`，保留原 readonly、coercion、owner revalidation 與 typed-reference 規則，沒有追加先前已被原生否定的 coercion 後 readonly 重檢。adapter 只接受依名稱選到的實體宣告，明確祖先 private descriptor 與完整公開 Reflection 驗證仍未實作，不能把私有 unsupported-descriptor 錯誤當成 PHP 診斷。

remaining 計數與旗標分開保存。proxy factory 執行中的 raw 寫入清除旗標，但 PHP 不會遞減計數，失敗後保留兩者差異。只有本次原先 lazy、標記最後仍有效、pending 且非 running 的外部 raw 操作才減一次，不能用 map 長度或掃描重新推導。這也省去每次完成判定掃描所有位置的成本，不宣稱整個名稱 resolver 都因此變成常數時間。

最後一個位置完成時，先清除 pending 與 proxy 身分、移出 initializer，再釋放 initializer owner；其 destructor 可重入並觀察已提交的值。receiver 與 RHS 先解參照，持有實際物件而非可被改綁的 reference container。各自讀原 refcount 後才取得 hold，receiver 取得後再判斷 RHS，清理依 RHS、receiver 次序合併錯誤。個別 stack hold 取代會逃逸的固定 slice，保留既有 constructor temporary 契約。

running ghost 對所有位置回報 non-lazy，所以唯一 raw producer 不會修改同一份 map。初始化可保存原 map 指標，失敗還原，不需 `maps.Clone`；清理只設 nil、不 `clear` 原 map。initializer 內 raw 寫入後拋例外、再重試的雙寬度案例保護這個前提。若未來增加 producer 或清空原 map，須重新驗證共享快照的條件。proxy 不回滾 raw 旗標。現有 clone 先完成初始化，因此沒有未初始化 ghost 共用可變 map 的情況。

本機 `lazyObject` 由 144 B 變成 160 B，`value`／`object`／property table 維持 64／96／64 B。raw 生命週期相對原直接 slot fixture 多 272 B／2 次配置；舊初始化會丟 raw 值，不能算相容的效能勝出。普通宣告物件維持原配置量，部分時間樣本較慢。同樣通過 605 案的 map-clone／slice-hold 候選，raw 初始化多 400 B／3 次配置，正式版本已省去；只有 raw、不初始化時省 144 B／一次。原始樣本、兩種 overlay、編譯器輸出、18 個新增案例及限制見 `W-13-r7-review.json`。

## R8：依實體順序還原型別來源

原 rollback 對每個 reference 呼叫 `attachPropertySource`，每次重新掃描完整 schema，untyped reference 也會掃描。`buildClass` 已按 schema 次序排除 static／virtual 並建立固定位置，override 留在原位置，繼承 private 則各自保留。因此直接走訪一次描述，以遞增的實體索引還原值，不需要名稱查表或新增索引。nil schema 沒有宣告位置，繼續保留 guard。

每一欄仍依序 detach、釋放舊值及執行 callback、移回 snapshot owner、登錄已知型別來源。不能批次重建登錄，因為解構 callback 會看到先還原與尚未還原欄位的不同來源次序。兩個固定映像及 Go 的 typed／untyped 共用參照案例保護此行為。原 helper 還有五個 R5 測試建置呼叫，原樣移到測試檔，正式執行不再需要這個掃描。

initializer 只有一個 receiver hold，改用單一 `methodHold` 值，先記錄原計數是否為零再 retain。釋放、`running=false` 及錯誤回傳順序不變。編譯器確認 defer closure 不逃逸，六個基準格都少一次配置，B/op 約少 80 B。1,024 個獨立參照欄位的 typed／untyped 中位數分別由 917637／1673530 ns 降至 106889／54816 ns。來源列表本身與 callback 仍有原有成本，不把整條路徑泛稱線性。相容的前後 607 案、三次樣本與可重建 overlay 見 `W-13-r8-review.json`。

PHP fixture 原先以 `ReflectionProperty::isInitialized` 觀察 untouched private lazy 欄位，該查詢本身會觸發第三次初始化。兩個固定映像的標記追蹤證明失敗發生於結果觀察，與 alias 建立無關。改用不初始化的 `isLazy`，Go 另檢查原 slot 仍 undefined；原始失敗及帶來源的 trace 保留，沒有更動 production 來迎合錯誤 fixture。

## 驗證與尚未完成項目

### 等待使用者裁決的額外參考引擎不安全案例

`diagnostics/argument_destructor_conditional.php` 在弱 Stringable argument 轉換後，來源解構拋例外，再進入包含條件分支的 body。固定 arm64 映像以 139 結束，i386 在 120 秒後逾時。Go 的 `TestW13PendingArgumentDestructorSafety` 安全完成、保留解構例外並清空請求物件，這是宿主安全測試，不是 PHP 相容通過。測試只執行 fixture 指定的 `ok` 分支，其他分支未驗證。

原始輸入、雜湊與雙寬度結果位於 `W-13-argument-destructor-diagnostic.json`。這是前兩項核准之外的新案例，已請使用者另外裁決，目前不得因此合併 PR #73，也不計為通過。

`diagnostics/reference_unset_clone.php` 是另一個獨立輸入：`__toString` 先 unset global alias，再 clone 原物件。固定 arm64 exit 139，i386 回報 target SIGSEGV 後於 120 秒逾時。Go 以原 reference hold 保住有效位置，原物件與 clone 都得到轉換後的值，alias 名稱不會重建，shutdown 無殘留。`TestW13PendingReferenceCloneSafety` 只證明宿主安全，不能算成 PHP 相容通過。原始雙寬度結果見 `W-13-reference-clone-diagnostic.json`，目前等待第四項獨立裁決，前兩項核准不適用。

### 使用者核准保留安全修正的參考引擎不安全案例

`diagnostics/gc_weak_resurrection.php` 在解構期間，經 WeakReference 將另一個不相連的循環物件保存到外部變數。固定 arm64 映像在輸出 `saved=live` 後以 139 結束。固定 i386 映像以 0 結束，但回報收集 2 個物件、外部變數仍是物件、WeakReference 已不再指向該物件。原始命令、輸出與 fixture 雜湊位於 `docs/implementation/evidence/W-13-gc-weak-diagnostic.json`。

Go 的 `TestW13GCExternalRootDuringDestructor` 已重現收集器釋放新增 owner 所指物件的問題。修正是在 destructor 改變圖之後重新核對存活，避免留下有效 owner 指向已釋放物件。此測試證明宿主安全不變條件，不是上述 PHP 案例的相容通過。

使用者於 2026-11-02 明確核准「保留安全修正，明列限制後允許合併」。本例保持相容性受阻，不能計為通過。此授權只適用本案例，其他尚未證明一致的行為仍須驗證或另行裁決。PR #73 必須先完成其餘驗收、審查與 CI 才能合併。

PHP 行為只由指定的 arm64 與 i386 digest 映像產生。CI 重新產生參考 JSON，先逐位元比對已提交的輸出，再用相同 guest 寬度執行 Go 操作矩陣。容器無網路、唯讀、移除 capabilities 並限制程序與記憶體，fixture 只有測試資料。

方法與 static 分派、hook／lazy 重入、解構與 WeakMap 矩陣、取消／大量循環及配置量測已有局部證據。完整 PHP frontend／VM、autoload／namespace、Closure／Reflection 公開介面、真實 I/O、歷史版本與應用驗收仍未涵蓋。最終結果及未涵蓋項目以 [W-13](../implementation/W-13.md) 及其執行證據為準。
