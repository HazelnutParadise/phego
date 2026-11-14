# Runtime 內部配置

本紀錄延續 [W-12 陣列](W-12-arrays.md)、[W-13 物件](W-13-objects.md) 與 W-14 執行框架，對應 REQ-012、REQ-016、REQ-017、REQ-018、REQ-020、REQ-022 與 T-023、T-024、T-031～T-036、T-039、T-040、T-043、T-044。這些改動都是 PHP 看不到的內部配置；PHP 8.5.12 的輸出、順序、訊息與比較次數不變，既有的 PHP 對照測試全部照舊通過。每一項都先量測改前與改後，沒有變好的不採用。

## 量測方式

改前與改後各自編成測試執行檔，交替執行四輪、每輪三次，取中位數，避免機器負載變化影響比較。配置次數與位元組在同一份程式碼上每次都相同；時間有雜訊，所以時間差異只在中位數與最小值方向一致時才採信。原始輸出在 [runtime-layout 證據目錄](../implementation/evidence/runtime-layout-20261107)。量測環境為 go1.26.5、darwin/arm64。

## 決定

| 項目 | 決定 | 量測結果 | 未採用的選項與理由 |
|---|---|---|---|
| 物件表 | 採用：`heap.objects []*object` 以 handle 為索引，另記存活數量，查詢時檢查邊界 | 物件建立與釋放快 39～72%，解構快 75%，循環回收快 78%，位元組與配置次數不變（[heap-objects](../implementation/evidence/runtime-layout-20261107/heap-objects.bench.txt)） | `map[uint64]*object`：handle 本來就密集且後進先出重用，map 只多了雜湊成本 |
| 陣列 bucket | 採用：由 heap 共用的塊配置，依序 5、13、21 個 bucket | 3、8、20 個元素的陣列快 6～7%，配置從 7、13、27 次降到 4、5、7 次，位元組大致不變；1,024 個元素的 append、COW 分離與排序配置少 94～95%，COW 快 20%，位元組多 1～2%（[bucket-chunks](../implementation/evidence/runtime-layout-20261107/bucket-chunks.bench.txt)） | 每個元素各配置一次：配置次數與元素數相同。每個陣列自己的塊（1、2、4、8、16 或其他序列）：多出的 slice 標頭讓 `phpArray` 從 96 bytes 級距跳到 112，小陣列反而多 2～25% 位元組 |
| 排序原位置 | 採用：交換時一起移動的 `[]int32` | 位元組少 13～15%，每次排序少 2～4 次配置；全部相等的資料快 16%，一般資料時間持平，比較次數不變（[sort-ordinal](../implementation/evidence/runtime-layout-20261107/sort-ordinal.bench.txt)） | `map[*bucket]int`：每次排序先建 map，平手時查兩次。放進 `arrayKey` 的空隙：重新編號的排序會把鍵換回原位，序號就跟不上值。放進 bucket：每個元素永久多 16 bytes |
| 執行框架 | 採用：`x.frames` 以值存放，同一深度重用 locals 的儲存空間 | 三層巢狀、每層兩個 local 的呼叫快 25%，配置從 15 次降到 6 次（[execution-frames](../implementation/evidence/runtime-layout-20261107/execution-frames.bench.txt)） | 每次呼叫配置新的 frame。locals 改成真正的值堆疊：locals 的位置交給呼叫端，擴容會讓舊指標失效，要等 VM 提供每個函式的變數數量（W-17） |
| 輸出上限的剩餘量 | 不採用：維持每次寫入掃描全部緩衝 | 增量記錄的版本在 1、3、8 層緩衝下量不出穩定差異（[output-write](../implementation/evidence/runtime-layout-20261107/output-write.bench.txt)） | 增量計數要在 5 個地方同步更新，沒有量到好處就不值得增加這份維護負擔 |

## CI 上的 benchstat 重量（2026-11-11）

上表的時間是在一台同時執行其他工作的 darwin/arm64 機器上取中位數，只能當方向參考。2026-11-11 改在 GitHub 的 ubuntu-24.04 runner（linux/amd64，go1.26.10，4 核）重量：每一項把改動前與改動後的 commit 各自編成測試執行檔，交替執行 10 輪，再以 benchstat 比較（`tools/benchstat-pair.sh`，「Runtime layout benchmarks」工作流程）。下表只列 benchstat 判定有差異（p < 0.05）的時間，配置次數與位元組和上表相同的不重複。原始輸出在[benchstat 證據目錄](../implementation/evidence/acceptance-followups-20261111/benchstat)。

| 項目 | 改動前 → 改動後 | CI 上的時間差異 | 和上表的差別 |
|---|---|---|---|
| 物件表 | 063aebe → 935a223 | 建立與釋放 0、1、4 個屬性的物件快 20%、10～11%、3～7%，32 與 256 個屬性量不出差異；materialize 後的生命週期快 10～13%，解構快 16.6%，循環回收快 13.0% | 方向一致，幅度小很多：上表的 39～72%、75%、78% 在 CI 上重現不出來 |
| 排序原位置 | 935a223 → 2cc3dcc | 全部相等的資料快 23.2%；32、128、1,024 個元素分別為 −3.0%、+2.5%、+0.5% | 一致：一般資料持平，相等資料變快 |
| 陣列 bucket | 2cc3dcc → e5eaac6 | COW 分離快 18.8%，1,024 個元素 append 快 3.7%，3、8、20 個元素快 1.5～2.6%；排序 32、128、1,024 個元素慢 6.2%、5.7%、7.3% | 小陣列只快 1.5～2.6%，不是 6～7%。排序變慢是上表沒有量到的退步 |
| 執行框架 | e5eaac6 → 81f9ac8 | 三層巢狀呼叫快 21.7% | 一致（上表為 25%） |

bucket 塊讓排序慢 6～7%，配置次數則少 78～94%。這項決定維持不變，因為配置次數與 COW 的改善涵蓋所有陣列操作，但排序的退步要在 W-17 以真實工作量重測時一併追查原因。

### echo 的 write 不和 writeTo 合併

`execution.write` 與 `writeTo` 的本體相同。驗收審查建議讓 `write` 直接呼叫 `writeTo`，依據是 [display_errors 的量測](../implementation/evidence/display-errors-20261107/bench-write.txt)，那份資料是在高負載的機器上取得。在 CI 上以 benchstat 重量（改動前為 f11fe9e）：

| 寫法 | 1、3、8 層緩衝的 `BenchmarkOutputWrite` |
|---|---|
| `write` 呼叫 `writeTo` | 慢 30.8%、33.1%、32.6% |
| 兩者都是一行、可內聯的入口，共用一個本體 | 慢 10.1%、9.7%、15.0% |
| 維持兩份本體（採用） | 基準 |

`completion` 是很大的結構，多經過一層函式回傳就有成本，所以保留兩份本體，並在程式註解寫明原因。

## Bucket 塊的細節

- Go 1.22 起，大於 512 bytes 且含指標的物件會多一個 8 bytes 的配置標頭。bucket 是 96 bytes，所以 8 個（768 bytes）實際占 896 的級距，16 個（1,536 bytes）占 1,792。選用的 5、13、21 個分別是 480、1,280（含標頭）與 2,048 bytes（含標頭），都剛好填滿級距。
- heap 只屬於一個請求，所以塊不需要鎖，最多活到請求結束。bucket 配置後不會移動，元素位置維持有效。
- 移除的 bucket，以及釋放陣列（含循環回收）時的每個 bucket，都會清空，位置不重複使用。塊由不同陣列共用，不清空的話，同一塊裡還活著的 bucket 會讓已釋放的值一直留在 Go 記憶體中（審查實測：交錯建立 200 個 1 MiB 的暫存陣列，修正前留住 201.6 MiB）。foreach by-reference 在綁定元素前先取出鍵，和 PHP 的 FE_FETCH_RW 相同，因為綁定時執行的解構子可能移除並清空目前的 bucket。塊在最後一個存活的 bucket 釋放後才會被回收，所以一個存活的 bucket 最多讓 20 個已移除的 bucket 留在記憶體中。
- 第一個塊只有 5 個 bucket，小請求不需要先付出大塊的成本。

## amd64 參考

64 位元原本只對照 arm64。在 linux/amd64 的 PHP 8.5.12 固定映像上重新產生四套參考後，只有 W-13 的 `compare-lazy-operand-owners` 不同：amd64 和 i386 一樣先初始化右邊的 lazy object，arm64 先初始化左邊（[amd64-reference](../implementation/evidence/runtime-layout-20261107/amd64-reference.txt)）。W-12、W-14、W-15 的輸出和 arm64 逐位元組相同。

- 參考 profile 改為（整數寬度、架構）。當時 `newHeap(width)` 使用該寬度的參考架構，`newHeapFor(width, arch)` 指定架構；沒有固定映像的組合會回報錯誤。系統後來也納入 profile，見 [macOS 與 Windows 的 profile](#macos-與-windows-的-profile)。
- W-13 另存 `w13-ref-64-amd64.json`，W-14 另存 `reference-amd64.json`（58 案和 arm64 相同，只有映像紀錄不同）。W-12、W-15 的 amd64 工作在原生 x86-64 runner 重新產生參考，先要求和 64 位元參考逐位元組相同，再執行 Go 對照：W-12 明確指定 amd64 profile，W-15 透過下一點的主機預設取得 amd64 profile。
- 一般執行（`newExecution`）建立的 64 位元 heap 依主機架構選 profile：amd64 主機用 amd64，其他用 arm64，32 位元用 386。嵌入 Phego 的程式在哪個平台跑，lazy 比較的初始化順序就和該平台的 PHP 8.5.12 相同。和已提交參考比對的測試則固定使用產生參考的架構（`referenceHeap`），不受測試主機影響。
- 產生工具新增 `--arch`；預設架構維持原本的檔名，其他架構的檔名加上架構後綴。

## macOS 與 Windows 的 profile

linux/amd64 的參考只來自 GCC 建置的映像。2026-11-11 在 GitHub runner 上用 `tests/semantics/w13-objects/lazy-operand-order.php` 重跑同一個案例（[host-profile 證據](../implementation/evidence/acceptance-followups-20261111/host-profile)）：

| 系統與 PHP 8.5.12 建置 | 先初始化的 lazy object |
|---|---|
| linux/arm64，固定映像（GCC） | 左邊 |
| linux/amd64、linux/386，固定映像（GCC） | 右邊 |
| macOS 15 arm64，鎖定原始碼以 Apple clang 17 建置 | 左邊 |
| macOS 15 amd64（Intel），鎖定原始碼以 Apple clang 17 建置 | 左邊 |
| Windows Server 2025 x64，官方 NTS 建置（Visual C++ 2022） | 右邊 |

macOS 的 amd64 和 linux/amd64 相反，所以參考 profile 改為（整數寬度、架構、系統）。`newHeap(width)` 在 macOS 與 Windows x64 上使用該系統的 profile，其他沒有實測的系統（例如 FreeBSD、Windows 的 32 位元或 arm64）沿用同架構的 Linux profile，這部分沒有對照過 PHP。`newHeapFor(width, arch, system)` 對沒有實測的組合回報錯誤。「Host profile on macOS and Windows」工作流程每次都重跑 PHP 並和 Phego 的主機 profile 比對（`TestHostProfileLazyOperandOrder`）。順序取決於建置 PHP 的 C 編譯器，同一系統改用其他編譯器建置的 PHP 可能不同，這裡只對照上表的建置。

## 後續

- `local()` 登記時會掃描所有框架的所有 local，這是目前每次呼叫真正的主要成本。改成只檢查目前框架會縮小重複登記的檢查範圍，留到 W-17 接上 VM、確定登記方式後再決定。
