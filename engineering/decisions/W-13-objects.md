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

有 `__toString` 的類別加入不可變 Stringable interface 描述，繼承與 trait 共用同一套型別匹配。enum 以四值 `enumKind` 區分普通類別、unit、int-backed、string-backed，填入 classSpec 原有 padding，R3 當時實測為 184 B。這能表示零 case enum，也能在建置時驗證 backing 數量／型別。`name` 與 `value` 納入 readonly 宣告布局，UnitEnum／BackedEnum 描述一併加入；建立 case 時只初始化一次，不再使用無屬性限制的動態位置。

回傳型別驗證放在方法自身的 W-14 callback 內，先完成轉換，再移交回傳 owner，最後釋放局部值。需轉型的 raw 結果由 `checkMethodReturn` 暫時持有並消耗一次。普通 PHP 轉換例外在 return 邊界由 TypeError 取代並保留 previous；argument 邊界保留原例外。成功的 argument 轉換若在釋放來源時解構拋例外，仍執行方法 body 並保留 pending 事件。取消、exit 與 internal failure 不使用這條繼續執行規則。

連到 typed property 的按參照回傳，值不符合回傳型別時，在轉換前拒絕，包含 mixed／union 來源與 int→float，避免改寫來源或執行 `__toString`。合法 callable 先做請求內符號／scope 驗證，屬於原樣接受，不會被這個限制誤認為需轉型。

方法表保存建置後不可變的 descriptor 指標。新定義與 trait 套用各有自己的 descriptor，純繼承共享原 descriptor；trait 綁定只複製描述本身，已凍結的參數與 defaults 可以共享。這讓方法實作身分可直接比較，也省掉原先每次 lookup 複製 descriptor 的配置。固定本機微型測試中，單次方法呼叫從 208 B／4 次配置降至 112 B／3 次配置。再把只保存單一 receiver 的 slice 改成一個 `methodHold` 值，降到 32 B／2 次配置。這批前後樣本執行時有其他工作，不能用其時間宣稱速度提升。

最後兩次配置來自 union／intersection 拆分的暫存 `[]string`。Go 的 `iter.Seq[string]` 直接逐段產生原字串切片，保留空 token、括號深度與提前停止規則，不增加快取或描述欄位。五次前後微型量測將無參數、回傳 int 的呼叫降至 0 B／0 次配置；全套 433 案及既有 DNF 控制維持一致。scanner 狀態物件也可省配置，但呼叫端較繁，現有 iterator 已達到目標，因此未採用。量測只支持這個操作，不推論所有方法或應用零配置。

WeakMap 沿用固定 PHP 的空標準屬性比較，兩個不同實例的 entries、順序、數量與循環均不影響相等結果。不同 class 仍不可比較。以 builtin identity 在 owner、materialization 與 recursion bookkeeping 前直接回傳相等，因為這條路徑不讀值也不執行 callback。這保留既有入口驗證，不新增另一套內容比較；Codex 建議的 entries 比較已被雙寬度原生區辨否定。

backed getter 在自身 frame 呼叫 isset 時直接檢查 backing 是否 undefined／null，避免轉成未初始化讀取錯誤。magic call 打包參數時保存是否已遇到具名參數，後續位置參數立即拒絕，不執行 `__call`。輸入對應 `call_user_func_array`，原始碼的 unpacking 診斷仍由 frontend／VM 整合驗收。

virtual setter-only 的 isset 走既有 readObject 錯誤路徑，backed setter 的自身 isset 保持 false。裸物件 callable 直接讀有效類別的 `__invoke` handler，不套 caller visibility 或 `__call`；明確 method-array callable 保留 magic 分流。private／protected 宣告警告由獨立原生探針保存，完整 frontend 裸物件呼叫與宣告診斷未驗收。

scoped private 查找以一次 ASCII 正規化及既有 method map 取代整表掃描，private owner 規則不變，不增加索引或快取。小寫 public、1024 個無關方法的五次量測從約 6.3 微秒降至 19 奈秒且零配置。大寫 private 的三次 overlay 量測新增 8 B／一次配置，從約 3.1 微秒降至 32 奈秒；接受此字串成本以免線性掃描，不宣稱全面零配置。各大小、無 scope 控制及重播方法見 `W-13-r4-review.json`。

類別 destructor adapter 只捕捉 callback 本身。Go `-gcflags=all=-m=2` 證明從以 reference 捕捉當時 184 B 的完整 classSpec，改成以 value 捕捉 8 B 的函式值。沒有新增狀態或改變 callback 行為，原始編譯器結果保存在 `W-13-destructor-capture.txt`。

執行 scalar 或 Stringable 轉換前先保護 receiver 與全部實參，避免前一個參數的 PHP callback 釋放後續參數。綁定完成後先解除原 operand owner，已轉成 scalar 的來源可在方法 body 前解構，其他值由參數 owner 接手。參數按宣告順序釋放，包含以相反順序傳入的具名參數。呼叫端不用回傳值時，還必須保存其 operand 存活範圍，先捨棄結果，再清理呼叫端暫存值；不能把使用與不使用結果的兩種輸入混為一談。

hook、lazy initializer 與正式類別 destructor 接到 W-14 的 frame／completion 邊界，沿用取消、深度限制、局部值釋放、fatal 與例外次序。回傳值在 frame 清理前取得 owner，離開後移交暫存值，避免 callback 回傳的物件在交付前被解構。既有 prototype helper 與公開 Go API 的名稱、簽章不變。

hook bypass 只看目前 PHP frame 的 receiver 與 property prototype。heap 保存一份目前 hook 狀態，真正呼叫函式時保存、清空，返回時還原。這省去每物件的 property-hook map 與 hook stack，也能讓 getter 經 helper 再讀同一屬性時重新進入 getter。父類別非 private 屬性的 override 保留 prototype identity。proxy hook 存取自己的 backing instance 時也視為相同 receiver。一般 finally 沿用原 frame，不另外清空狀態。

不同原始類別的物件比較直接回傳不可比較，不初始化 lazy object。需要比較內容時，先持有左右 operands，清理按左、右次序進行。指定映像對 lazy properties 的初始化次序不同：arm64 是左、右，i386 與 amd64 都是右、左。差異來自 C 編譯器對呼叫參數的求值順序，和整數寬度無關，所以 profile 以（整數寬度、架構）區分，`comparisonPropertiesPHP8512` 依 heap 的架構選擇次序（`newHeapFor`）。每個次序只是對應映像的證據，不能推到其他平台。上游 [zend_object_handlers.c:2352](https://github.com/php/php-src/blob/136e049ac10b032c83018e65b63b6df858943750/Zend/zend_object_handlers.c#L2352) 在同一個 C 呼叫求值兩個 properties 引數，原生 fixture 的雙寬度結果位於 `review.php`。

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

## R9：分批回收空的動態 block

刪除動態屬性後，舊版只解除值與 index，block 仍按歷史保留。空表反覆建立／刪除 `x` 十萬次會保留 100000 個位置與 6254 個 block，遍歷、clone 及清理也掃描這段歷史。位置可能已逃逸，不能搬動活位置或重用刪除位置。

選擇只回收完全沒有 index 成員的 block，存活判斷使用 index 的位置身分，不依值是否 undefined。每次成功動態 remove 累積飽和 uint32 計數，達到 `live+16` 向上對齊 16 的門檻後才掃描。門檻最多為 MaxUint32，先轉 uint64 再運算。掃描後歸零，以新增刪除量攤銷成本，不因稀疏活 block 的舊洞反覆掃描。空表兩次掃描間最多保留 15 個退休位置，每個保留的非空 block 最多 16 格。活屬性數改變及回收間隔仍會影響短暫保留量，不能把這個說明當成即時零保留。

最初 `live+16` 未對齊時，一個活屬性會讓每 17 次刪除丟棄兩個 16 格 block，量測由約 88 B/op 增為 165 B/op。改用對齊門檻不增加狀態，三種控制組都回到約 88 B/op。容量 hint 記住既有最大 block 容量，上限 16，避免全空回收後重新從 1 成長；它不允許重用舊位置。每項退休名稱及空 block 的 next 會解除，讓逃逸的刪除位置不再保留後續鏈，但不覆寫該位置的值。

`each` 與直接 hash comparison 在整段 callback 期間保護鏈。進入時保存原 bool，defer 還原，只有最外層出口能回收，巢狀或 panic 都不會提早解除外層保護。rollback 把容量及退休計數隨 dynamic storage 轉移，原 wrapper 的遍歷狀態保留。暫存清理 wrapper 保守維持保護，避免另一個 wrapper 的出口斷掉舊游標鏈。兩個 Go 內部指標控制在正常版本通過，移除比較 guard 或暫存 wrapper 保護後各自失敗。它們不算新增 PHP oracle，也沒有宣稱新的原生 comparison mutation 行為已驗收。

此策略不新增每個 entry 的 owner pointer 或每個 block 的 prev／live 欄位。每次 remove 全掃、依總洞數決定重掃、直接移動活位置，以及遍歷中 unlink 都不採用。立即清空每個零 live 表也會讓每次 churn 配置 block，因此保留分批門檻。活動 callback 可暫時累積歷史，保留量約束在最外層遍歷返回後成立。既有 Go map 容量不縮小，記憶體仍可能維持過往活屬性的高水位。

可讀 metadata 放進 Go/arm64 原 padding，表頭仍為 64 B，entry／block 不變。Go/386 表頭 32→36 B，十個合併配置物件生命週期格的 B/op 與次數不變，獨立 lazy snapshot 因配置級距多 16 B、次數不變。位元打包會改 materialized consumers、增加狀態操作並縮小計數範圍，這項成本不值得只為免除 Go/386 snapshot 的局部代價而引入。Go/386 的結構大小、配置量與回歸均已實跑，模擬環境時間只作原始紀錄。

普通及空歷史路徑增加 guard 成本，部分量測較慢；churn 也仍有分批配置。保留量及歷史掃描的改善不能抹去這些輸項。所有前後三次樣本、原始失敗、游標對照、對齊候選、可重建 overlay 與 609 案局部對照見 `W-13-r9-review.json`。

## R10：代理轉接、初始化存活與固定持有

代理物件的 wrapper descriptor 不必與相容父類別 backing 的位置相同。只換物件指標會沿用錯誤 storage key，甚至讀到 nil。先保留 wrapper 的權限與 hook dispatch，只有實際 backing resolver 回傳不同物件時，才重做完整 target descriptor、權限、hook、readonly 與位置解析。提前從已初始化 proxy 入口轉接會略過 wrapper getter，既有原生控制已重現此回歸，因此撤除。30 個控制另涵蓋冷／暖 setter、by-reference getter 與 hooked isset。

參照解析回傳實體 slot 與 descriptor，或 magic／getter 的 raw 回傳值。所有 forwarding guards 由 resolver 的 Go 呼叫堆疊還原後，外層才綁定目的地，讓舊目的地 destructor 正常重入。兩者之間只有 guard 還原，沒有可釋放 raw temporary 的 PHP callback。不新增 heap 狀態、不把綁定放進遞迴 resolver，也保留既有 target guard 的所有權。

lazy initializer 的 receiver hold 清理後，檢查的是原 wrapper 是否真正 freed，而非回傳的 backing 或零計數 constructor temporary。只在既有錯誤為 nil 時補上 `Lazy object was released during initialization`，保留 initializer、destructor 與 host stop 的優先順序。直接變數失去最後 owner 的五種屬性操作都報此錯誤；`$GLOBALS[...]` 的 read／isset 則有 VM temporary owner。測試 adapter 明確建模這個 operand hold，沒有暗改核心借用契約。

一般 declared、dynamic 與 static 屬性的最終 PHP store 傳入呼叫端 strictness，raw adapter 共用 declared store。Reflection raw 寫入在 strict PHP helper 中仍採 weak coercion，兩種固定映像已確認。內部 `assign` 與 typed-reference by-reference 參數的嚴格前置檢查保留，不能把所有 assign 呼叫一律替換。

static-as-instance notice 以當次 receiver class 組訊息。proxy 可以合法先印 Child 再印 Parent，不能去重。對應 magic method 只要存在就抑制這筆 lookup notice，與 guard 是否阻止 callback 無關。operation-aware optional 參數維持既有 private 呼叫可用，額外方法查找只在 static 分支執行；raw／lazy 查詢及 isset 保留 silent 行為。已初始化 proxy 的 unset 也先走 wrapper magic。共用小型 warning helper 省去重複格式來源，沒有新增物件狀態。

比較只需兩個持有值，hook 也只有 receiver 與可選 RHS，改用個別 stack `methodHold`，省去會逃逸的固定 slice。比較持有原左右 operand，清理左再右；hook 先 retain receiver，再讀 raw RHS 的原計數，保留同 identity 與 reference container。回傳值先受保護，再依 RHS、receiver、result 次序清理並合併每筆例外。真正可變長度的方法參數與 operand 保留現有 slice，不為這個局部改善新增泛用抽象。

Go/arm64 比較 144 B／一次降至零配置；getter 88 B／兩次降至 8 B／一次，scalar／object setter 的 80／224 B 降至零。Go/386 比較 96 B／一次降至零，getter 56→8 B、兩次→一次，兩種 setter 的 48／144 B 也降至零。getter 名稱仍需配置；把標籤快取到共享 metadata 會增加描述或建置成本，目前沒有宣稱消除此成本。此輪不更改 value、物件或屬性表布局。

每種寬度 809 案、原 609 案不變，實際 Go/386 回歸與前後三次量測見 `W-13-r10-review.json`。所有新輸入在兩個固定映像正常結束，沒有新增不安全參考引擎例外。第三與第四項既有診斷已於 2026-11-03 取得使用者明確核准，保留 Go 安全行為並明列限制後可合併，仍不計相容通過。

## R11：類別註冊、相對 callable 與 trait 組合

類別登錄遇到同一份已發布祖先即停止。已發布鏈在登錄時已完整驗證，且描述不可變；沿用這個條件可移除平方重掃，不新增索引或永久狀態。未發布部分保留取消檢查、名稱衝突與整批發布。1,024 類別三次樣本由約 54.7 MB／54 萬次配置降至約 158.5 KB／3,096 次。

Enum 與非物件比較先保留 bool／null 規則，其餘交由 enum unordered 規則，兩個方向都回傳 1，不執行普通物件的數值或字串轉換。一般物件及同 identity enum 的既有路徑保持原語意。

Callable 驗證以區域 `callableContext` 保存 callee lexical scope、實際 called class 與 `$this` 是否存在，不在 heap 或共享類別加狀態。relative class 成功解析後只記一次 deprecation，再判斷方法是否合法。回傳的 scalar／Stringable 轉換共用不重複驗證 callable 的尾段，保留原持有及例外順序。字串與陣列形式保留 PHP 的 `ce_org` 差異，array self 可查到實際 receiver 的方法時，不將同一規則套到 compound string。union 的其他成員原樣接受時不產生 callable deprecation。

Trait 組合用現有屬性索引範圍識別本次匯入，只有同 consumer 的碰撞才嚴格檢查並保留第一份描述。setter 權限依固定 PHP 保留第一份，不參與 trait 相容性遮罩。PHP 先組合 trait 再繼承，因此父類別非 private 宣告走既有覆寫路徑，保留 prototype 並建立 consumer owner；重新使用 trait 的 static 儲存因此獨立。祖先 private 維持各自實體位置。新增 permanent origin metadata 沒有必要。

型別比較處理 nullable、無序 union／intersection、self／parent、大小寫與 iterable 別名。Default 使用嚴格值比較，保留 NaN 不相等、空 bytes、陣列順序及重複正規化鍵的最後值。無平台的私人 schema 採 64 位元正規化，另掃描所有原始項目以保留兩端 32 位元可匯入性，包括最後被覆寫的項目；不用臨時 request heap，也不重做整套 32 位元遞迴比較。這是私人輸入可攜性條件，不宣稱完整的各平台 PHP 宣告接受條件。合法 float default 的最外層 int widening 保留，`int|float` 的整數與巢狀陣列的整數不改成 float。

Trait 首次建構取得獨立 snapshot，consumer 共用不可變內容，實例另行匯入請求值。省掉重複深複製後，256 組巢狀預設陣列的 consumer 建置由 32,936 B／1,041 次降至 912 B／15 次配置。原始輸入修改與平行請求中的巢狀寫入都不會影響其他 consumer。不為此建立快取或每屬性的共享旗標。

R11 共 482 個新增原生案例，兩種固定映像各 1,291 案且原 809 案完全不變。一般非法繼承宣告、autoload／alias 型別及 throwing deprecation handler 的完整 frontend／VM 流程未驗收。原始失敗、局部控制、樣本與重跑命令見 `W-13-r11-review.json`。

## R12：hook 參照、magic handler 與 setter 參數

外部取得 hooked property 參照時，先執行 getter。by-value object 結果可成為獨立參照，不能把 backing slot 暴露給外部寫入。其他值依固定 PHP 拒絕間接修改，typed UNDEF 保留先報未初始化的順序。setter-only 的 virtual 屬性不初始化 lazy object，自己的 hook backing 存取仍可 bypass。錯誤中的類別名稱使用實際 receiver。沿用既有 raw reference 解析結果，沒有新增參照種類或物件狀態。

`__call` 的有效 handler 依 receiver 選取，隱含呼叫不套一般方法權限。明確但不可存取的 `__call` 也能再次走 magic fallback。可正常存取的 parent private 方法保持 lexical 查找，callable 的 magic fallback 使用相同規則。六個非 public magic 宣告警告單獨保存，Go frontend 宣告診斷保持 `not_run`。

setter 參數可比 property 型別寬。例如 `int` backing 搭配 `set(int|string $incoming)`，輸入 `"12"` 以 string 進 body，再由 body 存入長度 2。參數名稱與型別放在 classSpec 的稀疏不可變 map，第一次建構複製輸入，trait 匯入在 owner 改綁前取得資料，繼承透過 declaring owner 查找。沒有額外 metadata 時保持 nil，不複製祖先 map，也不在每個 propertyInfo 增加兩個 string。classSpec 由 184 B 增至 192 B，共用類別描述有這筆成本，value／object／property table 布局不變。

shared hook worker 在轉型前持有 receiver 與原始 RHS。傳值參數先 dereference，驗證成功後才啟用 own-hook bypass，避免 `__toString` 間接存取同一屬性時繞過限制。`writeObjectAt` 保留原呼叫檔案與行號，舊 `writeObject` 入口不變，proxy forwarding 繼續傳遞位置。清理沿用 raw RHS、receiver、受保護結果的順序。專用 `setPropertyHook` 移除無用途的 hook 參數，沒有另外建立一般參數綁定陣列或新增請求狀態。

合成 setter 方法名稱原本讓成功呼叫多配置 8 B，實測後改成只在 TypeError 路徑建立。三次最終樣本中，scalar／object setter 維持 0 B／0 次配置，getter 保持 8 B／1 次。錯誤顯示型別只在失敗時依 PHP 的 named-first、builtin mask、nullable 與交集括號順序產生，不改寫共用型別資料。完整宣告型別變異、autoload／alias 與其他 frontend 合法性檢查保持未驗收，不以私人描述接受條件取代 PHP 編譯診斷。

兩種固定映像各 1,566 案，R12 新增 275 案且原 1,291 案完全不變。四項已核准的不安全原生輸入維持相容性受阻且不計通過。本輪沒有新增相容性例外。紅燈、原始觀察、命令與量測見 `W-13-r12-review.json`。

## R13：介面圖、延後診斷與資源重入

`classIs` 與名稱型別查詢原先遞迴展開每條介面路徑，菱形繼承的失敗查詢重複拜訪相同節點，22 層已需約 65～111 ms。描述建構只指向先前已完成的類別或 intrinsic，因此它們是不可變、無循環的圖。正式版保留 parent-only 與單一 successor 的直接迴圈，分支改用迭代深度優先走訪。只有所有介面與 parent 都失敗後才記住該節點，保留先命中的成功路徑零配置。nil schema 只結束該分支，不會略過其他待查節點。手動修改私人描述形成循環不屬於此建構契約。

走訪先使用 stack 上的 32 個 frame 與 8 個失敗節點，超過後才由 Go slice／pointer map 成長。狀態只屬於當次查詢，不保存在類別或請求快取。30 個固定 PHP 案例涵蓋父類別、重複介面、名稱大小寫、前導反斜線、Stringable 與 40／80 層；Go 另區分指標身分、同名描述、nil 及空名稱控制。這驗證 runtime 判斷核心，公開 `is_a` 的 frontend／VM 整合仍為 `not_run`。

比較三種做法。候選 A 在進入節點時記錄 visited，22 層成功與失敗都需 2,504 B／9 次配置。候選 B 為每個類別永久建立祖先閉包，鏈狀描述可能累積平方筆數，未實作或量測。正式候選 C 只記失敗節點，先命中路徑零配置，22 層失敗為 1,848 B／6 次配置及約 1.5～1.6 μs。較晚命中的名稱路徑相對 A 有小幅時間代價，單一介面鏈的身分查詢也有較慢樣本。22 層第一路徑成功的身分查詢由約 63～65 ns 變成 135～138 ns，單一 32 層介面鏈則由約 99 ns 變成 113～115 ns。原始全部樣本包含一筆 15,098 ns 的高值，不能宣稱每格都勝出。接受有限的失敗快取成本以移除指數展開，不加入永久 metadata 或另一層泛用圖抽象。

getter 沒有型別或恰為 mixed 時，在原持有與 invocation wrapper 內直接回傳 raw。typed getter 的暫時 `methodSpec` 沿用屬性名稱，新增 bool 標記，僅在 TypeError／never 錯誤格式化時組合 `$name::get`。原本先建標籤讓成功 getter 配置 8 B，延後後普通 getter 基準及八組成功控制為零配置，包含 weak string-to-int。既有參照、轉型、錯誤訊息與清理順序維持雙寬度對照。旗標使用原 padding，實測 arm64 的 methodSpec 88 B、386 的 44 B 都不增加。沒有每屬性名稱快取、建構期 wrapper 或成功路徑字串配置。

資源在執行 close callback 前先提交 closed／freed／close=nil，再取得 identity 暫時持有。持有直到 callback 及 panic 的 Stringer 格式化結束後才釋放，避免兩個 owner 交替期間短暫歸零造成有效 handle 提早刪除。最後一次 release 依 callback 結束後的 refcount 決定是否移除 registry。明確 close 若原有 owner 全部釋放也移除；原先為零的 constructor temporary 則保留到呼叫端 discard。關閉狀態不回復、callback 不重跑，cleanup context、取消及 shutdown 已關閉請求的限制保留。

72 組 Go 宿主控制涵蓋兩種寬度、三個入口、三種 owner 變化與成功／錯誤／panic／可重入 panic Stringer。它們不是新增 PHP 案例。原始版本、先釋放暫時 hold 再格式化的中間候選都曾失敗，正式版通過；實際 Go/386 回歸亦通過。兩個固定 PHP 映像各 1,596 案，原 1,566 案完全不變，四項已核准的不安全指定輸入保持受阻且不計通過，沒有新增例外。完整紅燈、原始觀察、來源雜湊、量測與重跑命令見 [R13 證據](../implementation/evidence/W-13-r13-review.json)。

## 驗證與尚未完成項目

### 第三、第四項已核准的指定輸入

`diagnostics/argument_destructor_conditional.php` 在弱 Stringable argument 轉換後，來源解構拋例外，再進入包含條件分支的 body。固定 arm64 映像以 139 結束，i386 在 120 秒後逾時。Go 的 `TestW13PendingArgumentDestructorSafety` 安全完成、保留解構例外並清空請求物件，這是宿主安全測試，不是 PHP 相容通過。測試只執行 fixture 指定的 `ok` 分支，其他分支未驗證。

原始輸入、雜湊與雙寬度結果位於 `W-13-argument-destructor-diagnostic.json`。使用者於 2026-11-03 另外明確核准：保留 Go 安全行為，將這個指定輸入明列相容性受阻、不計通過，其餘驗收與最新審查全數通過後允許合併 PR #73。此核准不擴及其他輸入或尚未執行的分支。

`diagnostics/reference_unset_clone.php` 是另一個獨立輸入：`__toString` 先 unset global alias，再 clone 原物件。固定 arm64 exit 139，i386 回報 target SIGSEGV 後於 120 秒逾時。Go 以原 reference hold 保住有效位置，原物件與 clone 都得到轉換後的值，alias 名稱不會重建，shutdown 無殘留。`TestW13PendingReferenceCloneSafety` 只證明宿主安全，不能算成 PHP 相容通過。原始雙寬度結果見 `W-13-reference-clone-diagnostic.json`，使用者於 2026-11-03 另行明確核准保留 Go 安全行為並明列本例限制，其餘驗收與最新審查全數通過後允許合併。此核准只適用記錄的輸入，不擴及其他差異。

### 使用者核准保留安全修正的參考引擎不安全案例

`diagnostics/gc_weak_resurrection.php` 在解構期間，經 WeakReference 將另一個不相連的循環物件保存到外部變數。固定 arm64 映像在輸出 `saved=live` 後以 139 結束。固定 i386 映像以 0 結束，但回報收集 2 個物件、外部變數仍是物件、WeakReference 已不再指向該物件。原始命令、輸出與 fixture 雜湊位於 `engineering/implementation/evidence/W-13-gc-weak-diagnostic.json`。

Go 的 `TestW13GCExternalRootDuringDestructor` 已重現收集器釋放新增 owner 所指物件的問題。修正是在 destructor 改變圖之後重新核對存活，避免留下有效 owner 指向已釋放物件。此測試證明宿主安全不變條件，不是上述 PHP 案例的相容通過。

使用者於 2026-11-02 明確核准「保留安全修正，明列限制後允許合併」。本例保持相容性受阻，不能計為通過。此授權只適用本案例，其他尚未證明一致的行為仍須驗證或另行裁決。PR #73 必須先完成其餘驗收、審查與 CI 才能合併。

PHP 行為只由指定的 arm64 與 i386 digest 映像產生。CI 重新產生參考 JSON，先逐位元比對已提交的輸出，再用相同 guest 寬度執行 Go 操作矩陣。容器無網路、唯讀、移除 capabilities 並限制程序與記憶體，fixture 只有測試資料。

方法與 static 分派、hook／lazy 重入、解構與 WeakMap 矩陣、取消／大量循環及配置量測已有局部證據。完整 PHP frontend／VM、autoload／namespace、Closure／Reflection 公開介面、真實 I/O、歷史版本與應用驗收仍未涵蓋。最終結果及未涵蓋項目以 [W-13](../implementation/W-13.md) 及其執行證據為準。
