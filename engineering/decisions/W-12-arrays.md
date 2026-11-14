# W-12 陣列、參照與迭代位置

本紀錄延續 [W-06 值模型](/engineering/decisions/W-06-value-model.md)，對應 REQ-007、REQ-016、REQ-017 與 T-013、T-014、T-031、T-032、T-033、T-034。只記錄 PHP 8.5.12 的 runtime 操作，不是完整 PHP 引擎或產品驗收證明。

## 選定做法

| 項目 | 選定做法 | 未採用的選項與理由 |
|---|---|---|
| packed／ordered | 延用穩定 bucket 指標，packed 允許有界空洞，混合鍵使用有序 bucket 與 Go map 索引 | 單獨 Go map 無順序；值 slice 會讓元素參照在成長後失效 |
| 複製與參照 | 寫入才分離，保留外部共享參照，單獨擁有的參照在分離時解開 | 一律深拷貝破壞別名；一律共享破壞一般賦值隔離 |
| by-value foreach | 持有原陣列，依序將值賦給迴圈位置 | 先深拷貝會看不到既有參照元素的更新 |
| by-ref foreach | 為候選副本保存獨立位置，取值時不提早分離目前陣列，選定副本後清除其他候選 | 單一 `copiedFrom` 無法處理多次分離；每圈分離會誤共享上一輪元素或保留過時位置 |
| 內部指標 | 與 foreach 位置分開；讀取不分離，移動指標先分離 | 共用一個位置會讓 foreach 改動 current／key 的結果 |
| 位置範圍 | 獨立記錄 PHP 8.5.12 的位置範圍與壓縮觸發，保留必要的空洞及失效指標位置 | 直接使用 Go slice 容量或只記「已失效」無法重現後續插入重新啟用指標的時機 |
| 鍵與診斷 | 沿用已測的數字字串與浮點轉換，依讀寫／unset 區分 null 診斷 | 依 Go `int` 寬度轉換不能模擬 32 位元 PHP |
| 排序 | 使用者排序在副本執行並提交至保留的參數參照；內建排序先分離再就地排序。比較排程及穩定補救與 PHP 8.5.12 一致 | Go 通用排序改變比較順序與例外部分排列；透過 caller slot 提交會漏改已解除綁定的原別名 |

數字鍵超出已滿的 packed 位置範圍時，先擴大範圍再轉 ordered，負整數也走此規則。一般字串鍵、較早的洞與排序轉換不套用這個預先擴大規則，未滿的 packed 範圍也不套用。固定映像案例涵蓋轉換後排序與分離仍保留此差異。

壓縮空洞與分離副本使用不同的位置映射。packed 副本保留空洞，ordered 副本壓縮空洞，但保留 PHP 對洞內指標與舊末端迭代器的位置結果。位置可以超出當前元素範圍，後續插入仍可能重新啟用，不能一律夾到目前尾端。穩定 bucket 指標、Go map 與明確的範圍計數只實作可觀察結果，沒有移植 Zend 配置器或原生記憶體結構。

W-06 的單元測試原先要求「刪除一格必須退出 packed」，這是已更換的私有布局假設。W-12 移除該假設，改成更完整的刪除、查找、追加、重新插入與 ordered 表示結果等價檢查，PHP 對照預期沒有放寬。

直接物件的 foreach 換綁先持有下一個參照，在舊位置仍有有效擁有者時執行本次換綁必然觸發的解構，再完成換綁與釋放。一般 assign／bind 的順序不變。另保留 arm64 PHP 8.5.12 解構重入造成參考程序記憶體損毀的獨立診斷結果，這些失敗不算相容通過；完整物件圖生命週期仍由 W-13 整合。

## 布局與量測結果

Apple M3、darwin/arm64 的型別布局：value 64 B、reference 72 B、arrayKey 32 B、bucket 96 B、phpArray 80 B、arrayIterator 8 B。布局由 `go test ./internal/runtime -run '^TestW12Layout$' -v` 取得，沒有使用 unsafe。

候選版本重跑命令為 `go test -run '^$' -bench 'Benchmark(PackedAppend1K|CopyOnWriteSeparation1K|ReferenceBindAndWrite|W12OrderedLookup1K)$' -benchtime 2000x -count 3 ./internal/runtime`。基準固定為量測時的 `origin/main`，即 `ddccbe8cbd62c8a03f7647a9be77db48ec56d31c`，使用相同命令移除 `|W12OrderedLookup1K`，因該版本沒有這項新增基準。

| 操作 | 基準每次耗時 | W-12 每次耗時 | 配置 |
|---|---|---|---|
| 連續追加 1024 個整數 | 52～78 µs | 47～51 µs | 1040 次、約 128 KB，兩者相同 |
| 複製 1024 元素並寫入一格 | 44～49 µs | 25～28 µs | 基準 1037 次／約 128 KB；W-12 1026 次／約 108 KB |
| 綁定參照並寫入 | 56～125 ns | 46～49 ns | 1 次、80 B，兩者相同 |
| ordered 陣列讀取一格 | 本批未量測 | 52～57 ns | 0 次、0 B |

表格使用最後一批樣本。前一批與最後一批的耗時差異很大，背景負載沒有固定，不能據此宣稱更快或排除耗時退步。W-12 以補足語意為目的，沒有以這次量測主張效能最佳化成立。各批原始樣本、基準 commit、工具版本與雜湊保存在[執行證據](/engineering/implementation/evidence/W-12-runtime-php8512.json)。每元素一次配置的成本仍存在，沒有在本票重寫布局。

## 排序相容修正與量測

REQ-017、T-033、T-034 要求比較呼叫本身也相容，包含不一致與不具遞移性的比較。`array_sort_php8512.go` 以 Go 重寫 PHP 8.5.12 的排序排程：2～5 個元素的小陣列路徑、6～16 的插入排序、較長陣列的分割排序及 1024 元素起的五點取樣。相等比較以原始位置維持穩定。比較函式拋錯後不再呼叫使用者函式，但繼續執行內部穩定補救，保留 PHP 的部分排列。

`array_sort_user_php8512.go` 對 bool false 反向呼叫一次，保留巢狀排序的 deprecation 狀態。診斷掛點可傳遞 deprecation 或轉型警告處理函式拋出的例外。W-17（issue #21、PR #77）負責正式呼叫與錯誤處理函式接線。回傳參照包住 bool 時不做 bool 補救。W-13（issue #18、PR #73）負責尚未建立的物件／enum 內建比較處理。

排序內部比較接收元素位置。每次使用者函式呼叫都重新解開參照並持有當次值，反向重試不沿用前次快照。回傳值在釋放參數前先持有，直到轉型與診斷結束才釋放。若重試關閉 heap，回傳值仍清理，但禁止再呼叫宿主診斷，並優先保留原始 callback 錯誤。

callback 拋錯與參數清理拋錯分開傳遞。後者保留已產生的比較結果，並在待處理例外下抑制診斷與 false 重試。重試不額外改寫 deprecation 旗標，以保留巢狀排序的重設。對照命令與兩種位元寬度的結果存於證據 `codex_review_revision.additional_regressions`。

空副本保留下一索引，但不複製初始化狀態與迭代位置。`limit == 0` 是 Go 表示未初始化的記號，不是 Zend 欄位布局。內建排序的混合陣列移除空洞時保留原始迭代位置；packed 轉換則使用原有位置映射。這些行為與排序中暫時清空查找索引均有 PHP 對照。

上游版本鎖定 PHP 8.5.12 commit `136e049ac10b032c83018e65b63b6df858943750`。衍生程式保留 Zend 2.00、PHP 3.01 及 libc++ 的完整授權與聲明，見 [THIRD_PARTY_NOTICES.md](/THIRD_PARTY_NOTICES.md) 與 [UPSTREAM.json](/third_party/php-sort/UPSTREAM.json)。原創專案程式維持 MIT。

排序量測使用 `go test ./internal/runtime -run '^$' -bench '^BenchmarkW12Sorting$' -benchtime 200x -count 3`，輸入為固定種子的整數序列。基準為修正前 `65adcab` 的 Go 排序，候選與 PHP 固定映像的比較次數完全相同：

| 長度 | 原 Go 比較次數 | 修正後／PHP 比較次數 | 修正後配置 |
|---|---:|---:|---|
| 32 | 191 | 148 | 42 次，約 7.5 KB |
| 128 | 1119 | 810 | 138 次，約 29.5 KB |
| 1024 | 12938 | 9942 | 1038 次，約 243 KB |

比較次數沒有增加，穩定位置表增加配置。各批耗時受背景負載影響而大幅變動，不能據此主張加速。原始樣本、失敗再現及 650 組案例在兩種位元寬度的對照見[排序修正證據](/engineering/implementation/evidence/W-12-sorting-php8512.json)。這是 runtime 整數比較量測，不包含 PHP VM 或應用效能。

PHP 次數使用同一個 seed 73 數列，每次追加前以 `(seed*73+19)%9973` 更新，完整受限容器命令、內嵌 PHP 腳本及排序輸出的 JSON 雜湊編碼存於證據的 `native_comparison_probe`。從專案根目錄可重跑：

```python
import json, subprocess
with open("docs/implementation/evidence/W-12-sorting-php8512.json") as source:
    probe = json.load(source)["native_comparison_probe"]
subprocess.run(probe["command"], check=True)
```

## 已知限制

- 差異案例只涵蓋 PHP 8.5.12 的兩種整數寬度；其他歷史版本、作業系統差異與正式 VM／AOT 尚未驗證。
- 參照參數／回傳、型別限制及 resource 由後續物件／呼叫／資源工作整合，沒有回報完整 T-013～T-034 已通過。
- 多個物件解構例外的主例外替換與 previous 鏈尚未建立，由 W-13／W-17 整合，目前 heap 保留首次錯誤。
- 核心 heap 由單一請求擁有；未加入全域可變狀態、cgo、LLVM、Zend 正式回退、公開 Go 契約變更或新的付費依賴。
