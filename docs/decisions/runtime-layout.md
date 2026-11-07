# Runtime 內部配置

本紀錄延續 [W-12 陣列](W-12-arrays.md)、[W-13 物件](W-13-objects.md) 與 W-14 執行框架，對應 REQ-012、REQ-016、REQ-017、REQ-018、REQ-020、REQ-022 與 T-023、T-024、T-031～T-036、T-039、T-040、T-043、T-044。這些改動都是 PHP 看不到的內部配置；PHP 8.5.12 的輸出、順序、訊息與比較次數不變，既有的 PHP 對照測試全部照舊通過。每一項都先量測改前與改後，沒有變好的不採用。

## 量測方式

改前與改後各自編成測試執行檔，交替執行四輪、每輪三次，取中位數，避免機器負載變化影響比較。配置次數與位元組在同一份程式碼上每次都相同；時間有雜訊，所以時間差異只在中位數與最小值方向一致時才採信。原始輸出在 [runtime-layout 證據目錄](../implementation/evidence/runtime-layout-20261107/)。量測環境為 go1.26.5、darwin/arm64。

## 決定

| 項目 | 決定 | 量測結果 | 未採用的選項與理由 |
|---|---|---|---|
| 物件表 | 採用：`heap.objects []*object` 以 handle 為索引，另記存活數量，查詢時檢查邊界 | 物件建立與釋放快 39～72%，解構快 75%，循環回收快 78%，位元組與配置次數不變（[heap-objects](../implementation/evidence/runtime-layout-20261107/heap-objects.bench.txt)） | `map[uint64]*object`：handle 本來就密集且後進先出重用，map 只多了雜湊成本 |
| 陣列 bucket | 採用：由 heap 共用的塊配置，依序 5、13、21 個 bucket | 3、8、20 個元素的陣列快 6～7%，配置從 7、13、27 次降到 4、5、7 次，位元組大致不變；1,024 個元素的 append、COW 分離與排序配置少 94～95%，COW 快 20%，位元組多 1～2%（[bucket-chunks](../implementation/evidence/runtime-layout-20261107/bucket-chunks.bench.txt)） | 每個元素各配置一次：配置次數與元素數相同。每個陣列自己的塊（1、2、4、8、16 或其他序列）：多出的 slice 標頭讓 `phpArray` 從 96 bytes 級距跳到 112，小陣列反而多 2～25% 位元組 |
| 排序原位置 | 採用：交換時一起移動的 `[]int32` | 位元組少 13～15%，每次排序少 2～4 次配置；全部相等的資料快 16%，一般資料時間持平，比較次數不變（[sort-ordinal](../implementation/evidence/runtime-layout-20261107/sort-ordinal.bench.txt)） | `map[*bucket]int`：每次排序先建 map，平手時查兩次。放進 `arrayKey` 的空隙：重新編號的排序會把鍵換回原位，序號就跟不上值。放進 bucket：每個元素永久多 16 bytes |
| 執行框架 | 採用：`x.frames` 以值存放，同一深度重用 locals 的儲存空間 | 三層巢狀、每層兩個 local 的呼叫快 25%，配置從 15 次降到 6 次（[execution-frames](../implementation/evidence/runtime-layout-20261107/execution-frames.bench.txt)） | 每次呼叫配置新的 frame。locals 改成真正的值堆疊：locals 的位置交給呼叫端，擴容會讓舊指標失效，要等 VM 提供每個函式的變數數量（W-17） |
| 輸出上限的剩餘量 | 不採用：維持每次寫入掃描全部緩衝 | 增量記錄的版本在 1、3、8 層緩衝下量不出穩定差異（[output-write](../implementation/evidence/runtime-layout-20261107/output-write.bench.txt)） | 增量計數要在 5 個地方同步更新，沒有量到好處就不值得增加這份維護負擔 |

## Bucket 塊的細節

- Go 1.22 起，大於 512 bytes 且含指標的物件會多一個 8 bytes 的配置標頭。bucket 是 96 bytes，所以 8 個（768 bytes）實際占 896 的級距，16 個（1,536 bytes）占 1,792。選用的 5、13、21 個分別是 480、1,280（含標頭）與 2,048 bytes（含標頭），都剛好填滿級距。
- heap 只屬於一個請求，所以塊不需要鎖，最多活到請求結束。bucket 配置後不會移動，元素位置維持有效。
- 移除的 bucket 會清空，位置不重複使用；塊在最後一個存活的 bucket 釋放後才會被回收，所以一個存活的 bucket 最多讓 20 個已移除的 bucket 留在記憶體中。
- 第一個塊只有 5 個 bucket，小請求不需要先付出大塊的成本。

## amd64 參考

64 位元原本只對照 arm64。在 linux/amd64 的 PHP 8.5.12 固定映像上重新產生四套參考後，只有 W-13 的 `compare-lazy-operand-owners` 不同：amd64 和 i386 一樣先初始化右邊的 lazy object，arm64 先初始化左邊（[amd64-reference](../implementation/evidence/runtime-layout-20261107/amd64-reference.txt)）。W-12、W-14、W-15 的輸出和 arm64 逐位元組相同。

- 參考 profile 改為（整數寬度、架構）。`newHeap(width)` 照舊使用該寬度的參考架構（64 位元為 arm64、32 位元為 386），`newHeapFor(width, arch)` 指定架構；沒有固定映像的組合會回報錯誤。
- W-13 另存 `w13-ref-64-amd64.json`。W-12、W-15 的 amd64 工作在原生 x86-64 runner 重新產生參考，先要求和 64 位元參考逐位元組相同，再用 amd64 profile 執行 Go 對照。
- 產生工具新增 `--arch`；預設架構維持原本的檔名，其他架構的檔名加上架構後綴。

## 後續

- `local()` 登記時會掃描所有框架的所有 local，這是目前每次呼叫真正的主要成本。改成只檢查目前框架會縮小重複登記的檢查範圍，留到 W-17 接上 VM、確定登記方式後再決定。
