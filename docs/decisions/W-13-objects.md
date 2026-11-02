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

Go 1.26.5、darwin/arm64、Apple M3 的最後三次微型量測，value 為 64 B、bucket 96 B、object 96 B、propertyTable 56 B。32 屬性表的 W-12 陣列布局為 8880 B／47 次配置，宣告表為 2304 B／1 次，動態表為 7704 B／15 次。含物件建立、寫入與釋放的結果如下，原始 main 為 `4578556dcba800b34a9f0ad8a0cc8359e4e6e0d1`。

| 屬性數 | 原始 main 動態物件 B／配置 | W-13 動態物件 B／配置 | W-13 宣告物件 B／配置 |
|---|---:|---:|---:|
| 0 | 160／2 | 160／1 | 160／1 |
| 1 | 664／6 | 544／4 | 224／2 |
| 4 | 1000／11 | 1136／6 | 416／2 |
| 32 | 9040／49 | 7864／16 | 2464／2 |
| 256 | 72624／282 | 51224／36 | 18592／2 |

4 個動態屬性比原始 main 多 136 B，但少 5 次配置。這個固定位置與成長容量的成本明列保留，不能宣稱所有情境記憶體都下降。32 與 256 屬性的成本支持採用專用布局，沒有 PHP VM、真實應用或競品效能通過的含義。原始樣本、基準原始碼與重跑命令列在 W-13 evidence。

## 參照限制與請求狀態

型別限制按實際屬性位置附加，別名寫入同一 reference 時一起驗證。unset、物件釋放、clone 與循環釋放必須維護來源的加入與解除。限制放在 heap 的按需側表，避免每個 reference 都增加一個 slice header。value 維持 64 B；為拒絕跨請求的活陣列與 reference，各自增加一個 heap owner 指標，phpArray 為 88 B、reference 為 80 B。

對尚未認領的 array／reference 圖，先驗證所有可達值，整體成功後才提交 owner。已認領的同請求值使用固定成本檢查，不重走整張圖。bind 與 foreach 綁定會先保存兩個可寫位置的原始零計數狀態，再各取得所需 owner，避免兩位置指向同一個 constructor-style temporary 時，先取得的計數掩蓋另一位置的需要。普通 Go value 複製本身不取得 PHP owner。

類別描述不保存物件欄位、static 值、enum 活實例、lazy 初始化錯誤或 hook 重入狀態。這些狀態由該請求的 heap／object 擁有。Go callback 是受信任的宿主程式；程式描述中的 callback 不應捕捉 PHP 請求的可變值。

resource 使用既有 identity 指標欄位與獨立的請求登錄，不在每個 PHP value 加一個 resource 指標。使用時核對 owner、種類、identity 與代次。關閉先提交 closed 狀態，再呼叫宿主清理器，防止重入重複關閉。PHP scope 釋放與 W-14 最終清理共用該登錄，實際 I/O 與能力授權仍由對應模組處理。

## 呼叫與生命週期

方法表保存建置後不可變的 descriptor 指標。新定義與 trait 套用各有自己的 descriptor，純繼承共享原 descriptor；trait 綁定只複製描述本身，已凍結的參數與 defaults 可以共享。這讓方法實作身分可直接比較，也省掉原先每次 lookup 複製 descriptor 的配置。固定本機微型測試中，單次方法呼叫從 208 B／4 次配置降至 112 B／3 次配置。再把只保存單一 receiver 的 slice 改成一個 `methodHold` 值，降到 32 B／2 次配置。這批前後樣本執行時有其他工作，不能用其時間宣稱速度提升。

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

## 驗證與尚未完成項目

### 使用者核准保留安全修正的參考引擎不安全案例

`diagnostics/gc_weak_resurrection.php` 在解構期間，經 WeakReference 將另一個不相連的循環物件保存到外部變數。固定 arm64 映像在輸出 `saved=live` 後以 139 結束。固定 i386 映像以 0 結束，但回報收集 2 個物件、外部變數仍是物件、WeakReference 已不再指向該物件。原始命令、輸出與 fixture 雜湊位於 `docs/implementation/evidence/W-13-gc-weak-diagnostic.json`。

Go 的 `TestW13GCExternalRootDuringDestructor` 已重現收集器釋放新增 owner 所指物件的問題。修正是在 destructor 改變圖之後重新核對存活，避免留下有效 owner 指向已釋放物件。此測試證明宿主安全不變條件，不是上述 PHP 案例的相容通過。

使用者於 2026-11-02 明確核准「保留安全修正，明列限制後允許合併」。本例保持相容性受阻，不能計為通過。此授權只適用本案例，其他尚未證明一致的行為仍須驗證或另行裁決。PR #73 必須先完成其餘驗收、審查與 CI 才能合併。

PHP 行為只由指定的 arm64 與 i386 digest 映像產生。CI 重新產生參考 JSON，先逐位元比對已提交的輸出，再用相同 guest 寬度執行 Go 操作矩陣。容器無網路、唯讀、移除 capabilities 並限制程序與記憶體，fixture 只有測試資料。

方法與 static 分派、hook／lazy 重入、解構與 WeakMap 矩陣、取消／大量循環及配置量測已有局部證據。完整 PHP frontend／VM、autoload／namespace、Closure／Reflection 公開介面、真實 I/O、歷史版本與應用驗收仍未涵蓋。最終結果及未涵蓋項目以 [W-13](../implementation/W-13.md) 及其執行證據為準。
