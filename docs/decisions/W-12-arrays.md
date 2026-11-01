# W-12 陣列、參照與迭代位置

本紀錄延續 [W-06 值模型](W-06-value-model.md)，對應 REQ-007、REQ-016、REQ-017 與 T-013、T-014、T-031、T-032、T-033、T-034。只記錄 PHP 8.5.12 的 runtime 操作，不是完整 PHP 引擎或產品驗收證明。

## 選定做法

| 項目 | 選定做法 | 未採用的選項與理由 |
|---|---|---|
| packed／ordered | 延用穩定 bucket 指標，連續整數鍵免索引，混合鍵使用有序 bucket 與 Go map 索引 | 單獨 Go map 無順序；值 slice 會讓元素參照在成長後失效 |
| 複製與參照 | 寫入才分離，保留外部共享參照，單獨擁有的參照在分離時解開 | 一律深拷貝破壞別名；一律共享破壞一般賦值隔離 |
| by-value foreach | 持有原陣列，依序將值賦給迴圈位置 | 先深拷貝會看不到既有參照元素的更新 |
| by-ref foreach | 每個迭代器為各存活副本保存獨立位置，釋放陣列與結束迭代時解除登錄 | 單一 `copiedFrom` 只能辨認一次分離，也會保留整條舊陣列鏈 |
| 內部指標 | 與 foreach 位置分開；讀取不分離，移動指標先分離 | 共用一個位置會讓 foreach 改動 current／key 的結果 |
| 鍵與診斷 | 沿用已測的數字字串與浮點轉換，依讀寫／unset 區分 null 診斷 | 依 Go `int` 寬度轉換不能模擬 32 位元 PHP |
| 排序 | 在獨立副本穩定排序；callback 參數在呼叫期間持有所有權，結束後提交副本 | 原地排序會讓 callback 重入破壞結構；拋例外就保留原陣列不符本版參考結果 |

## 布局與量測結果

Apple M3、darwin/arm64 的型別布局：value 64 B、reference 72 B、arrayKey 32 B、bucket 96 B、phpArray 72 B、arrayIterator 8 B。布局由 `go test ./internal/runtime -run '^TestW12Layout$' -v` 取得，沒有使用 unsafe。

候選版本重跑命令為 `go test -run '^$' -bench 'Benchmark(PackedAppend1K|CopyOnWriteSeparation1K|ReferenceBindAndWrite|W12OrderedLookup1K)$' -benchtime 2000x -count 3 ./internal/runtime`。基準固定為量測時的 `origin/main`，即 `ddccbe8cbd62c8a03f7647a9be77db48ec56d31c`，使用相同命令移除 `|W12OrderedLookup1K`，因該版本沒有這項新增基準。

| 操作 | 基準每次耗時 | W-12 每次耗時 | 配置 |
|---|---|---|---|
| 連續追加 1024 個整數 | 52～78 µs | 48～55 µs | 1040 次、約 128 KB，兩者相同 |
| 複製 1024 元素並寫入一格 | 44～49 µs | 27～28 µs | 1037 次、約 128 KB，兩者相同 |
| 綁定參照並寫入 | 56～125 ns | 46～48 ns | 1 次、80 B，兩者相同 |
| ordered 陣列讀取一格 | 本批未量測 | 116～157 ns | 0 次、0 B |

表格使用最後一批樣本。前一批與最後一批的耗時差異很大，背景負載沒有固定，不能據此宣稱更快或排除耗時退步。W-12 以補足語意為目的，沒有以這次量測主張效能最佳化成立。兩批原始樣本、基準 commit、工具版本與雜湊保存在[執行證據](../implementation/evidence/W-12-runtime-php8512.json)。每元素一次配置的成本仍存在，沒有在本票重寫布局。

## 已知限制

- 差異案例只涵蓋 PHP 8.5.12 的兩種整數寬度；其他歷史版本、作業系統差異與正式 VM／AOT 尚未驗證。
- callback 比較次數與順序不是本票保證；已驗證穩定結果、鍵、別名、修改原陣列及首次比較拋例外的狀態。
- 參照參數／回傳、型別限制及 resource 由後續物件／呼叫／資源工作整合，沒有回報完整 T-013～T-034 已通過。
- 核心 heap 由單一請求擁有；未加入全域可變狀態、cgo、LLVM、Zend 正式回退、公開 Go 契約變更或新的付費依賴。
