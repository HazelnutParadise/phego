# Phego 全部實作計畫與 GitHub 追蹤

本計畫把規格1.1.2的60個工作包逐一對應到issue、獨立分支及draft PR，涵蓋100項需求與200個驗收情境。PR目前提交可審查的實作計畫，後續程式與測試沿同一分支加入。

## 現況與下一個工作包

產品情境維持 `not_run`，引擎尚未實作。原始資料仍保存初始規劃狀態，W-03 須建立可驗證的狀態轉移。建立 issue、開 draft 或合併契約文件都不構成產品驗收。

[W-01](W-01.md) 的契約與威脅檢查已完成局部驗收，合併狀態以其 PR 為準。W-01 合併後可進行 [W-02](https://github.com/HazelnutParadise/phego/issues/2)，負責固定來源與工具基準。

目前引擎、來源鎖、歷史參考環境、競品實測及真實網站驗收均尚未完成。歷史來源或外部環境缺失必須在實作時登錄具體阻礙，不能據此縮小範圍。本輪規劃沒有軟體release或正式部署授權。

## 規格與實作流程

架構與測試規則沿用[完整規格](../spec/specification.zh-TW.md#chapter-04)、[決策登錄](../decisions/README.md)、[測試策略](../spec/specification.zh-TW.md#chapter-22)與[工作包資料](../data/phego-work-packages.json)。規劃來源提交為 `6e2e701174a2f8d7afb0a884305007923684f1a9`，精確來源摘要記錄於 [GitHub對照資料](github-index.json)。

```text
Go API / CLI / Apache、Nginx FastCGI / HTTP / 封裝程式
  → 有界接納 → 請求範圍的版本、政策與執行狀態
  → 解析與語意 IR → 驗證 Bytecode → Go VM
                    ↘ 經驗證的可選 LLVM AOT ↗
  → 共用語意與能力檢查 → 受控檔案、資料庫、網路與宿主 callback
  → 輸出、結果快照、診斷、收尾與證據
```

Engine與Program只共享已驗證的不可變資料，Execution擁有每請求的可變值、環境、Session視圖與失敗狀態。Site重載採設定代次，Result輸出採獨立快照。歷史前端共用正式語意與政策契約，不能另建一套會繞過安全檢查的引擎。

完整流程至少驗證：Go單函式執行與取消；Apache／Nginx上的登入、上傳及Session；封裝應用的唯讀程式與可寫資料；各歷史版本的合法應用配對；VM／AOT的相同副作用與取消結果。

## 測試責任與退出條件

每張票列出實作順序、產物、預定位置、錯誤與空值／零值／重複／並行／取消情境，以及原始驗收操作和預期結果。工具票先驗證自己的工具及分類行為，不能用替身結果宣稱PHP相容。

200個情境各有一張最終驗收負責票，記錄於 `github-index.json` 的 `acceptance_owner`。相關工作包提供局部測試及證據，負責票整合完整情境。原規格的多對多追蹤關係保留，指定負責票不會免除其他工作包的退出條件。

| 測試入口 | 需驗證的結果 | 外部效果處理 |
|---|---|---|
| 獨立來源、追蹤、PHPT工具 | 來源身分、分類、完整分母與失敗保留 | 參考引擎在拋棄式受限環境 |
| Go公開SDK與CLI | 輸出、return、錯誤、所有權、取消及Close | callback為受信任宿主程式，限制須明示 |
| 真實FastCGI／HTTP | 原始參數、路由、標頭、上傳、Session及斷線 | 使用測試資料及受控服務 |
| VM／AOT／混合執行 | 相同語意、一次性副作用、能力與安全資訊 | 動態程式回到Phego VM，禁止Zend生產回退 |
| 真實應用與歷史程式 | 官方測試、瀏覽器互動、CLI、資料與升級回復 | 支付、郵件及通知用替身或獲授權sandbox |
| 效能、故障與長測 | 正確性先行、逐格嚴格勝出、資源有界及回復 | 隔離資料副本，至少24小時及100萬請求 |

實際Go模組出現後，交付執行格式檢查、靜態檢查、核心及受影響profile測試，並依變更加上race、fuzz、ABI或真實應用測試。這些產品測試目前沒有執行。證據須包含commit、工具與環境、命令、輸入、原始結果、資料／資源差異及所有未知。

Ready for review前須補齊本票實作與測試。PR描述使用 `Refs` 連結issue，計畫文件不設定自動關閉issue的關鍵字。只有實際實作與本票退出條件成立才能關閉實作issue，完整產品驗收另看指定情境及W-60。

## 早期工作與完整驗收分開

- W-02在M0建立最小上游差異偵測：官方tag、stubs、parser、測試與公告來源清單，重跑命令及來源快照。最少在每次來源鎖更新與認證候選前執行；持續排程的頻率與實際設定須隨實作提交。W-45補上完整分類與多版本回歸。
- W-49與W-54在W-01～W-03完成後立即進行正式清冊與隔離環境工作。唯讀來源發現可更早並行。[歷史首版與早期版本線索](evidence/W-49-early-history.md)及[競品候選調查](evidence/W-54-competitor-discovery.md)保留原始出處及未知事項。PHP第一版來源缺失、競品無法建置、工作負載不可比都保留原狀態。
- W-03先整理真實應用來源與合法版本配對需求，W-34、W-56～W-58建立參考流程及預期資料。受測端沒有完成時保留預期失敗，不能改應用PHP來源。
- 階段標籤表示能力成熟階段，不是全域等待閘門。W-40、W-41所需能力完成後即可整合，W-56～W-58不必等其他無關M7工作。可選W-47、W-48沒有通過時保持停用。

## 整票整合的補充依賴

以下依賴由工作包已承諾的交付內容推得，與原始 `dependencies` 分開保存。原規格資料不改寫。可以先做不依賴這些能力的子步驟，整票驗收需等待相應依賴，Pure-Go預覽依規格24.2.1產生明示的適用子圖。

| 工作包 | 整合另需 | 理由 |
|---|---|---|
| W-35 | W-25 | serve／fastcgi 子命令必須接到實際協定入口 |
| W-35 | W-26 | serve 子命令必須接到實際 HTTP 入口 |
| W-41 | W-46 | 程序隔離終止與硬性限制測試需要真實 worker |
| W-41 | W-38 | 完整 native ABI／指標與 GC 檢查需要正式原生 Runtime |
| W-43 | W-21 | observe／enforce 效能比較需要實際來源追蹤與政策 |
| W-56 | W-36 | 宣告的封裝模式驗收需要實際封裝與 VFS |
| W-57 | W-36 | 第38章適用的單檔模式需要實際封裝 |
| W-58 | W-36 | 第38章適用的單檔模式需要實際封裝 |
| W-60 | W-45 | 認證截點的新版本與安全差異必須已完成追蹤與回歸 |

## 可並行的依賴批次

同批工作只有在各自前置條件已通過後才可開始。批次是完整整合順序，子步驟的先行準備依各票說明。這不是工期估算。

| 批次 | 工作包 |
|---|---|
| 1 | W-01 |
| 2 | W-02 |
| 3 | W-03 |
| 4 | W-04、W-05、W-49、W-54 |
| 5 | W-06、W-07、W-09 |
| 6 | W-08、W-10 |
| 7 | W-11、W-20、W-45 |
| 8 | W-12、W-14 |
| 9 | W-13、W-15、W-16 |
| 10 | W-17、W-21 |
| 11 | W-18、W-19、W-22、W-29、W-37、W-50 |
| 12 | W-23、W-24、W-30、W-51 |
| 13 | W-25、W-26、W-31、W-32、W-33、W-38、W-46 |
| 14 | W-27、W-35、W-39、W-52 |
| 15 | W-28、W-36 |
| 16 | W-34、W-40、W-41 |
| 17 | W-43、W-53、W-56、W-57、W-58 |
| 18 | W-42、W-47、W-48、W-55、W-59 |
| 19 | W-44 |
| 20 | W-60 |

## 全部issue與draft PR

| 工作包 | 階段 | 原規格工作 | 規格及整合依賴 | Issue | Draft PR | 詳細計畫 |
|---|---|---|---|---|---|---|
| W-01 | M0 | 建立產品契約與威脅模型 | 無 | [#1](https://github.com/HazelnutParadise/phego/issues/1) | [#61](https://github.com/HazelnutParadise/phego/pull/61) | [計畫](https://github.com/HazelnutParadise/phego/blob/main/docs/implementation/W-01.md) |
| W-02 | M0 | 鎖定官方與工具基準 | [W-01](https://github.com/HazelnutParadise/phego/issues/1) | [#2](https://github.com/HazelnutParadise/phego/issues/2) | [#62](https://github.com/HazelnutParadise/phego/pull/62) | [計畫](https://github.com/HazelnutParadise/phego/blob/main/docs/implementation/W-02.md) |
| W-03 | M0 | 建立追蹤與證據工具 | [W-01](https://github.com/HazelnutParadise/phego/issues/1)、[W-02](https://github.com/HazelnutParadise/phego/issues/2) | [#3](https://github.com/HazelnutParadise/phego/issues/3) | [#63](https://github.com/HazelnutParadise/phego/pull/63) | [計畫](https://github.com/HazelnutParadise/phego/blob/main/docs/implementation/W-03.md) |
| W-04 | M0 | 建立官方測試與差異runner | [W-02](https://github.com/HazelnutParadise/phego/issues/2)、[W-03](https://github.com/HazelnutParadise/phego/issues/3) | [#4](https://github.com/HazelnutParadise/phego/issues/4) | [#64](https://github.com/HazelnutParadise/phego/pull/64) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-04-implementation/docs/implementation/W-04.md) |
| W-05 | M1 | 生命週期與資源狀態模型 | [W-01](https://github.com/HazelnutParadise/phego/issues/1)、[W-03](https://github.com/HazelnutParadise/phego/issues/3) | [#5](https://github.com/HazelnutParadise/phego/issues/5) | [#65](https://github.com/HazelnutParadise/phego/pull/65) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-05-implementation/docs/implementation/W-05.md) |
| W-06 | M1 | 值／參照／生命週期原型 | [W-02](https://github.com/HazelnutParadise/phego/issues/2)、[W-04](https://github.com/HazelnutParadise/phego/issues/4)、[W-05](https://github.com/HazelnutParadise/phego/issues/5) | [#8](https://github.com/HazelnutParadise/phego/issues/8) | [#66](https://github.com/HazelnutParadise/phego/pull/66) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-06-implementation/docs/implementation/W-06.md) |
| W-07 | M1 | LLVM與Go橋接概念驗證 | [W-02](https://github.com/HazelnutParadise/phego/issues/2)、[W-05](https://github.com/HazelnutParadise/phego/issues/5) | [#9](https://github.com/HazelnutParadise/phego/issues/9) | [#67](https://github.com/HazelnutParadise/phego/pull/67) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-07-implementation/docs/implementation/W-07.md) |
| W-08 | M1 | 定義Go公開API與結果契約 | [W-01](https://github.com/HazelnutParadise/phego/issues/1)、[W-03](https://github.com/HazelnutParadise/phego/issues/3)、[W-06](https://github.com/HazelnutParadise/phego/issues/8) | [#11](https://github.com/HazelnutParadise/phego/issues/11) | [#68](https://github.com/HazelnutParadise/phego/pull/68) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-08-implementation/docs/implementation/W-08.md) |
| W-09 | M2 | Lexer與parser | [W-02](https://github.com/HazelnutParadise/phego/issues/2)、[W-04](https://github.com/HazelnutParadise/phego/issues/4) | [#10](https://github.com/HazelnutParadise/phego/issues/10) | [#69](https://github.com/HazelnutParadise/phego/pull/69) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-09-implementation/docs/implementation/W-09.md) |
| W-10 | M2 | 版本規則與相容診斷 | [W-02](https://github.com/HazelnutParadise/phego/issues/2)、[W-09](https://github.com/HazelnutParadise/phego/issues/10) | [#12](https://github.com/HazelnutParadise/phego/issues/12) | [#70](https://github.com/HazelnutParadise/phego/pull/70) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-10-implementation/docs/implementation/W-10.md) |
| W-11 | M2 | 高階IR與呼叫語意 | [W-06](https://github.com/HazelnutParadise/phego/issues/8)、[W-09](https://github.com/HazelnutParadise/phego/issues/10)、[W-10](https://github.com/HazelnutParadise/phego/issues/12) | [#13](https://github.com/HazelnutParadise/phego/issues/13) | [#71](https://github.com/HazelnutParadise/phego/pull/71) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-11-implementation/docs/implementation/W-11.md) |
| W-12 | M2 | 正式array與reference實作 | [W-06](https://github.com/HazelnutParadise/phego/issues/8)、[W-11](https://github.com/HazelnutParadise/phego/issues/13) | [#16](https://github.com/HazelnutParadise/phego/issues/16) | [#72](https://github.com/HazelnutParadise/phego/pull/72) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-12-implementation/docs/implementation/W-12.md) |
| W-13 | M3 | 物件與邏輯生命週期 | [W-05](https://github.com/HazelnutParadise/phego/issues/5)、[W-06](https://github.com/HazelnutParadise/phego/issues/8)、[W-11](https://github.com/HazelnutParadise/phego/issues/13)、[W-12](https://github.com/HazelnutParadise/phego/issues/16) | [#18](https://github.com/HazelnutParadise/phego/issues/18) | [#73](https://github.com/HazelnutParadise/phego/pull/73) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-13-implementation/docs/implementation/W-13.md) |
| W-14 | M2 | Frame、錯誤與shutdown | [W-05](https://github.com/HazelnutParadise/phego/issues/5)、[W-08](https://github.com/HazelnutParadise/phego/issues/11)、[W-11](https://github.com/HazelnutParadise/phego/issues/13) | [#17](https://github.com/HazelnutParadise/phego/issues/17) | [#74](https://github.com/HazelnutParadise/phego/pull/74) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-14-implementation/docs/implementation/W-14.md) |
| W-15 | M3 | SSA、效果與安全保留最佳化 | [W-11](https://github.com/HazelnutParadise/phego/issues/13)、[W-12](https://github.com/HazelnutParadise/phego/issues/16)、[W-14](https://github.com/HazelnutParadise/phego/issues/17) | [#19](https://github.com/HazelnutParadise/phego/issues/19) | [#75](https://github.com/HazelnutParadise/phego/pull/75) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-15-implementation/docs/implementation/W-15.md) |
| W-16 | M2 | Bytecode格式與驗證器 | [W-11](https://github.com/HazelnutParadise/phego/issues/13)、[W-14](https://github.com/HazelnutParadise/phego/issues/17) | [#20](https://github.com/HazelnutParadise/phego/issues/20) | [#76](https://github.com/HazelnutParadise/phego/pull/76) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-16-implementation/docs/implementation/W-16.md) |
| W-17 | M2 | VM通用執行路徑 | [W-12](https://github.com/HazelnutParadise/phego/issues/16)、[W-14](https://github.com/HazelnutParadise/phego/issues/17)、[W-16](https://github.com/HazelnutParadise/phego/issues/20) | [#21](https://github.com/HazelnutParadise/phego/issues/21) | [#77](https://github.com/HazelnutParadise/phego/pull/77) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-17-implementation/docs/implementation/W-17.md) |
| W-18 | M3 | 動態載入、eval與autoload | [W-10](https://github.com/HazelnutParadise/phego/issues/12)、[W-13](https://github.com/HazelnutParadise/phego/issues/18)、[W-17](https://github.com/HazelnutParadise/phego/issues/21)、[W-20](https://github.com/HazelnutParadise/phego/issues/14) | [#23](https://github.com/HazelnutParadise/phego/issues/23) | [#78](https://github.com/HazelnutParadise/phego/pull/78) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-18-implementation/docs/implementation/W-18.md) |
| W-19 | M3 | Generator與Fiber | [W-13](https://github.com/HazelnutParadise/phego/issues/18)、[W-14](https://github.com/HazelnutParadise/phego/issues/17)、[W-17](https://github.com/HazelnutParadise/phego/issues/21) | [#24](https://github.com/HazelnutParadise/phego/issues/24) | [#79](https://github.com/HazelnutParadise/phego/pull/79) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-19-implementation/docs/implementation/W-19.md) |
| W-20 | M2 | 能力與配額核心 | [W-01](https://github.com/HazelnutParadise/phego/issues/1)、[W-05](https://github.com/HazelnutParadise/phego/issues/5)、[W-08](https://github.com/HazelnutParadise/phego/issues/11) | [#14](https://github.com/HazelnutParadise/phego/issues/14) | [#80](https://github.com/HazelnutParadise/phego/pull/80) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-20-implementation/docs/implementation/W-20.md) |
| W-21 | M5 | 來源追蹤與脈絡政策 | [W-12](https://github.com/HazelnutParadise/phego/issues/16)、[W-15](https://github.com/HazelnutParadise/phego/issues/19)、[W-20](https://github.com/HazelnutParadise/phego/issues/14) | [#22](https://github.com/HazelnutParadise/phego/issues/22) | [#81](https://github.com/HazelnutParadise/phego/pull/81) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-21-implementation/docs/implementation/W-21.md) |
| W-22 | M4 | Goroutine接納與Execution隔離 | [W-05](https://github.com/HazelnutParadise/phego/issues/5)、[W-14](https://github.com/HazelnutParadise/phego/issues/17)、[W-17](https://github.com/HazelnutParadise/phego/issues/21)、[W-20](https://github.com/HazelnutParadise/phego/issues/14) | [#25](https://github.com/HazelnutParadise/phego/issues/25) | [#82](https://github.com/HazelnutParadise/phego/pull/82) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-22-implementation/docs/implementation/W-22.md) |
| W-23 | M4 | 共享編譯快取 | [W-16](https://github.com/HazelnutParadise/phego/issues/20)、[W-17](https://github.com/HazelnutParadise/phego/issues/21)、[W-22](https://github.com/HazelnutParadise/phego/issues/25) | [#29](https://github.com/HazelnutParadise/phego/issues/29) | [#83](https://github.com/HazelnutParadise/phego/pull/83) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-23-implementation/docs/implementation/W-23.md) |
| W-24 | M4 | 正式SDK與HostCall | [W-08](https://github.com/HazelnutParadise/phego/issues/11)、[W-17](https://github.com/HazelnutParadise/phego/issues/21)、[W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-22](https://github.com/HazelnutParadise/phego/issues/25) | [#30](https://github.com/HazelnutParadise/phego/issues/30) | [#84](https://github.com/HazelnutParadise/phego/pull/84) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-24-implementation/docs/implementation/W-24.md) |
| W-25 | M4 | FastCGI adapter | [W-14](https://github.com/HazelnutParadise/phego/issues/17)、[W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-22](https://github.com/HazelnutParadise/phego/issues/25)、[W-24](https://github.com/HazelnutParadise/phego/issues/30) | [#33](https://github.com/HazelnutParadise/phego/issues/33) | [#85](https://github.com/HazelnutParadise/phego/pull/85) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-25-implementation/docs/implementation/W-25.md) |
| W-26 | M4 | 內建HTTP與公開目錄路由 | [W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-22](https://github.com/HazelnutParadise/phego/issues/25)、[W-24](https://github.com/HazelnutParadise/phego/issues/30) | [#34](https://github.com/HazelnutParadise/phego/issues/34) | [#86](https://github.com/HazelnutParadise/phego/pull/86) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-26-implementation/docs/implementation/W-26.md) |
| W-27 | M4 | PHP請求映射與上傳 | [W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-25](https://github.com/HazelnutParadise/phego/issues/33)、[W-26](https://github.com/HazelnutParadise/phego/issues/34) | [#40](https://github.com/HazelnutParadise/phego/issues/40) | [#87](https://github.com/HazelnutParadise/phego/pull/87) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-27-implementation/docs/implementation/W-27.md) |
| W-28 | M4 | Session與共享狀態 | [W-14](https://github.com/HazelnutParadise/phego/issues/17)、[W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-22](https://github.com/HazelnutParadise/phego/issues/25)、[W-27](https://github.com/HazelnutParadise/phego/issues/40) | [#44](https://github.com/HazelnutParadise/phego/issues/44) | [#88](https://github.com/HazelnutParadise/phego/pull/88) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-28-implementation/docs/implementation/W-28.md) |
| W-29 | M3 | 核心函式與模組登錄 | [W-10](https://github.com/HazelnutParadise/phego/issues/12)、[W-13](https://github.com/HazelnutParadise/phego/issues/18)、[W-17](https://github.com/HazelnutParadise/phego/issues/21)、[W-20](https://github.com/HazelnutParadise/phego/issues/14) | [#26](https://github.com/HazelnutParadise/phego/issues/26) | [#89](https://github.com/HazelnutParadise/phego/pull/89) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-29-implementation/docs/implementation/W-29.md) |
| W-30 | M5 | 字串、編碼與PCRE | [W-12](https://github.com/HazelnutParadise/phego/issues/16)、[W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-29](https://github.com/HazelnutParadise/phego/issues/26) | [#31](https://github.com/HazelnutParadise/phego/issues/31) | [#90](https://github.com/HazelnutParadise/phego/pull/90) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-30-implementation/docs/implementation/W-30.md) |
| W-31 | M5 | 資料庫與PDO／mysqli | [W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-24](https://github.com/HazelnutParadise/phego/issues/30)、[W-29](https://github.com/HazelnutParadise/phego/issues/26) | [#35](https://github.com/HazelnutParadise/phego/issues/35) | [#91](https://github.com/HazelnutParadise/phego/pull/91) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-31-implementation/docs/implementation/W-31.md) |
| W-32 | M5 | HTTP、crypto與網路政策 | [W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-24](https://github.com/HazelnutParadise/phego/issues/30)、[W-29](https://github.com/HazelnutParadise/phego/issues/26) | [#36](https://github.com/HazelnutParadise/phego/issues/36) | [#92](https://github.com/HazelnutParadise/phego/pull/92) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-32-implementation/docs/implementation/W-32.md) |
| W-33 | M5 | XML、封存、國際化與網站額外模組 | [W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-29](https://github.com/HazelnutParadise/phego/issues/26)、[W-30](https://github.com/HazelnutParadise/phego/issues/31) | [#37](https://github.com/HazelnutParadise/phego/issues/37) | [#93](https://github.com/HazelnutParadise/phego/pull/93) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-33-implementation/docs/implementation/W-33.md) |
| W-34 | M5 | Composer與網站認證場景 | [W-18](https://github.com/HazelnutParadise/phego/issues/23)、[W-25](https://github.com/HazelnutParadise/phego/issues/33)、[W-26](https://github.com/HazelnutParadise/phego/issues/34)、[W-27](https://github.com/HazelnutParadise/phego/issues/40)、[W-28](https://github.com/HazelnutParadise/phego/issues/44)、[W-29](https://github.com/HazelnutParadise/phego/issues/26)、[W-30](https://github.com/HazelnutParadise/phego/issues/31)、[W-31](https://github.com/HazelnutParadise/phego/issues/35)、[W-32](https://github.com/HazelnutParadise/phego/issues/36)、[W-33](https://github.com/HazelnutParadise/phego/issues/37) | [#46](https://github.com/HazelnutParadise/phego/issues/46) | [#94](https://github.com/HazelnutParadise/phego/pull/94) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-34-implementation/docs/implementation/W-34.md) |
| W-35 | M4 | CLI、設定與相容檢查入口 | [W-10](https://github.com/HazelnutParadise/phego/issues/12)、[W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-24](https://github.com/HazelnutParadise/phego/issues/30)、[W-29](https://github.com/HazelnutParadise/phego/issues/26)、[W-25](https://github.com/HazelnutParadise/phego/issues/33)、[W-26](https://github.com/HazelnutParadise/phego/issues/34) | [#41](https://github.com/HazelnutParadise/phego/issues/41) | [#95](https://github.com/HazelnutParadise/phego/pull/95) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-35-implementation/docs/implementation/W-35.md) |
| W-36 | M6 | 單檔封裝與VFS | [W-18](https://github.com/HazelnutParadise/phego/issues/23)、[W-23](https://github.com/HazelnutParadise/phego/issues/29)、[W-26](https://github.com/HazelnutParadise/phego/issues/34)、[W-35](https://github.com/HazelnutParadise/phego/issues/41) | [#45](https://github.com/HazelnutParadise/phego/issues/45) | [#96](https://github.com/HazelnutParadise/phego/pull/96) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-36-implementation/docs/implementation/W-36.md) |
| W-37 | M6 | LLVM降低與AOT產生 | [W-07](https://github.com/HazelnutParadise/phego/issues/9)、[W-15](https://github.com/HazelnutParadise/phego/issues/19)、[W-17](https://github.com/HazelnutParadise/phego/issues/21) | [#27](https://github.com/HazelnutParadise/phego/issues/27) | [#97](https://github.com/HazelnutParadise/phego/pull/97) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-37-implementation/docs/implementation/W-37.md) |
| W-38 | M6 | 正式Native Runtime ABI | [W-07](https://github.com/HazelnutParadise/phego/issues/9)、[W-14](https://github.com/HazelnutParadise/phego/issues/17)、[W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-24](https://github.com/HazelnutParadise/phego/issues/30)、[W-37](https://github.com/HazelnutParadise/phego/issues/27) | [#38](https://github.com/HazelnutParadise/phego/issues/38) | [#98](https://github.com/HazelnutParadise/phego/pull/98) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-38-implementation/docs/implementation/W-38.md) |
| W-39 | M6 | 後端等價與安全整合 | [W-15](https://github.com/HazelnutParadise/phego/issues/19)、[W-18](https://github.com/HazelnutParadise/phego/issues/23)、[W-19](https://github.com/HazelnutParadise/phego/issues/24)、[W-21](https://github.com/HazelnutParadise/phego/issues/22)、[W-38](https://github.com/HazelnutParadise/phego/issues/38) | [#42](https://github.com/HazelnutParadise/phego/issues/42) | [#99](https://github.com/HazelnutParadise/phego/pull/99) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-39-implementation/docs/implementation/W-39.md) |
| W-40 | M7 | 維運、重載、遷移與回復 | [W-22](https://github.com/HazelnutParadise/phego/issues/25)、[W-25](https://github.com/HazelnutParadise/phego/issues/33)、[W-26](https://github.com/HazelnutParadise/phego/issues/34)、[W-28](https://github.com/HazelnutParadise/phego/issues/44)、[W-35](https://github.com/HazelnutParadise/phego/issues/41) | [#47](https://github.com/HazelnutParadise/phego/issues/47) | [#100](https://github.com/HazelnutParadise/phego/pull/100) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-40-implementation/docs/implementation/W-40.md) |
| W-41 | M7 | 全系統fuzz、race與故障注入 | [W-05](https://github.com/HazelnutParadise/phego/issues/5)、[W-16](https://github.com/HazelnutParadise/phego/issues/20)、[W-17](https://github.com/HazelnutParadise/phego/issues/21)、[W-22](https://github.com/HazelnutParadise/phego/issues/25)、[W-25](https://github.com/HazelnutParadise/phego/issues/33)、[W-27](https://github.com/HazelnutParadise/phego/issues/40)、[W-28](https://github.com/HazelnutParadise/phego/issues/44)、[W-30](https://github.com/HazelnutParadise/phego/issues/31)、[W-46](https://github.com/HazelnutParadise/phego/issues/39)、[W-38](https://github.com/HazelnutParadise/phego/issues/38) | [#48](https://github.com/HazelnutParadise/phego/issues/48) | [#101](https://github.com/HazelnutParadise/phego/pull/101) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-41-implementation/docs/implementation/W-41.md) |
| W-42 | M7 | 完整相容性與網站驗收 | [W-04](https://github.com/HazelnutParadise/phego/issues/4)、[W-19](https://github.com/HazelnutParadise/phego/issues/24)、[W-29](https://github.com/HazelnutParadise/phego/issues/26)、[W-34](https://github.com/HazelnutParadise/phego/issues/46)、[W-39](https://github.com/HazelnutParadise/phego/issues/42)、[W-41](https://github.com/HazelnutParadise/phego/issues/48)、[W-52](https://github.com/HazelnutParadise/phego/issues/43)、[W-53](https://github.com/HazelnutParadise/phego/issues/50)、[W-56](https://github.com/HazelnutParadise/phego/issues/51)、[W-57](https://github.com/HazelnutParadise/phego/issues/52)、[W-58](https://github.com/HazelnutParadise/phego/issues/53) | [#54](https://github.com/HazelnutParadise/phego/issues/54) | [#102](https://github.com/HazelnutParadise/phego/pull/102) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-42-implementation/docs/implementation/W-42.md) |
| W-43 | M7 | 效能、容量與長時間測試 | [W-17](https://github.com/HazelnutParadise/phego/issues/21)、[W-22](https://github.com/HazelnutParadise/phego/issues/25)、[W-23](https://github.com/HazelnutParadise/phego/issues/29)、[W-34](https://github.com/HazelnutParadise/phego/issues/46)、[W-39](https://github.com/HazelnutParadise/phego/issues/42)、[W-40](https://github.com/HazelnutParadise/phego/issues/47)、[W-54](https://github.com/HazelnutParadise/phego/issues/7)、[W-21](https://github.com/HazelnutParadise/phego/issues/22) | [#49](https://github.com/HazelnutParadise/phego/issues/49) | [#103](https://github.com/HazelnutParadise/phego/pull/103) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-43-implementation/docs/implementation/W-43.md) |
| W-44 | M7 | 供應鏈與發布候選 | [W-02](https://github.com/HazelnutParadise/phego/issues/2)、[W-03](https://github.com/HazelnutParadise/phego/issues/3)、[W-36](https://github.com/HazelnutParadise/phego/issues/45)、[W-40](https://github.com/HazelnutParadise/phego/issues/47)、[W-42](https://github.com/HazelnutParadise/phego/issues/54)、[W-43](https://github.com/HazelnutParadise/phego/issues/49) | [#59](https://github.com/HazelnutParadise/phego/issues/59) | [#104](https://github.com/HazelnutParadise/phego/pull/104) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-44-implementation/docs/implementation/W-44.md) |
| W-45 | M8 | 上游版本追蹤 | [W-02](https://github.com/HazelnutParadise/phego/issues/2)、[W-03](https://github.com/HazelnutParadise/phego/issues/3)、[W-04](https://github.com/HazelnutParadise/phego/issues/4)、[W-10](https://github.com/HazelnutParadise/phego/issues/12) | [#15](https://github.com/HazelnutParadise/phego/issues/15) | [#105](https://github.com/HazelnutParadise/phego/pull/105) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-45-implementation/docs/implementation/W-45.md) |
| W-46 | M5 | 高風險worker隔離模式 | [W-05](https://github.com/HazelnutParadise/phego/issues/5)、[W-20](https://github.com/HazelnutParadise/phego/issues/14)、[W-22](https://github.com/HazelnutParadise/phego/issues/25)、[W-24](https://github.com/HazelnutParadise/phego/issues/30) | [#39](https://github.com/HazelnutParadise/phego/issues/39) | [#106](https://github.com/HazelnutParadise/phego/pull/106) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-46-implementation/docs/implementation/W-46.md) |
| W-47 | M8 | 可選JIT實驗 | [W-37](https://github.com/HazelnutParadise/phego/issues/27)、[W-38](https://github.com/HazelnutParadise/phego/issues/38)、[W-39](https://github.com/HazelnutParadise/phego/issues/42)、[W-43](https://github.com/HazelnutParadise/phego/issues/49) | [#55](https://github.com/HazelnutParadise/phego/issues/55) | [#107](https://github.com/HazelnutParadise/phego/pull/107) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-47-implementation/docs/implementation/W-47.md) |
| W-48 | M8 | 自適應接納與池化實驗 | [W-22](https://github.com/HazelnutParadise/phego/issues/25)、[W-23](https://github.com/HazelnutParadise/phego/issues/29)、[W-41](https://github.com/HazelnutParadise/phego/issues/48)、[W-43](https://github.com/HazelnutParadise/phego/issues/49) | [#56](https://github.com/HazelnutParadise/phego/issues/56) | [#108](https://github.com/HazelnutParadise/phego/pull/108) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-48-implementation/docs/implementation/W-48.md) |
| W-49 | M0 | 全歷史版本清冊與隔離參考環境 | [W-01](https://github.com/HazelnutParadise/phego/issues/1)、[W-02](https://github.com/HazelnutParadise/phego/issues/2)、[W-03](https://github.com/HazelnutParadise/phego/issues/3) | [#6](https://github.com/HazelnutParadise/phego/issues/6) | [#109](https://github.com/HazelnutParadise/phego/pull/109) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-49-implementation/docs/implementation/W-49.md) |
| W-50 | M2 | 早期 PHP Tools／PHP-FI 前端與工具語意 | [W-49](https://github.com/HazelnutParadise/phego/issues/6)、[W-11](https://github.com/HazelnutParadise/phego/issues/13)、[W-17](https://github.com/HazelnutParadise/phego/issues/21) | [#28](https://github.com/HazelnutParadise/phego/issues/28) | [#110](https://github.com/HazelnutParadise/phego/pull/110) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-50-implementation/docs/implementation/W-50.md) |
| W-51 | M3 | 完整版本與平台語意規則 | [W-49](https://github.com/HazelnutParadise/phego/issues/6)、[W-06](https://github.com/HazelnutParadise/phego/issues/8)、[W-07](https://github.com/HazelnutParadise/phego/issues/9)、[W-10](https://github.com/HazelnutParadise/phego/issues/12)、[W-19](https://github.com/HazelnutParadise/phego/issues/24) | [#32](https://github.com/HazelnutParadise/phego/issues/32) | [#111](https://github.com/HazelnutParadise/phego/pull/111) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-51-implementation/docs/implementation/W-51.md) |
| W-52 | M5 | 歷史內建函式與擴充相容 | [W-50](https://github.com/HazelnutParadise/phego/issues/28)、[W-51](https://github.com/HazelnutParadise/phego/issues/32)、[W-29](https://github.com/HazelnutParadise/phego/issues/26)、[W-30](https://github.com/HazelnutParadise/phego/issues/31)、[W-31](https://github.com/HazelnutParadise/phego/issues/35)、[W-32](https://github.com/HazelnutParadise/phego/issues/36)、[W-33](https://github.com/HazelnutParadise/phego/issues/37) | [#43](https://github.com/HazelnutParadise/phego/issues/43) | [#112](https://github.com/HazelnutParadise/phego/pull/112) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-52-implementation/docs/implementation/W-52.md) |
| W-53 | M5 | 混合版本並行與安全契約 | [W-49](https://github.com/HazelnutParadise/phego/issues/6)、[W-50](https://github.com/HazelnutParadise/phego/issues/28)、[W-51](https://github.com/HazelnutParadise/phego/issues/32)、[W-22](https://github.com/HazelnutParadise/phego/issues/25)、[W-23](https://github.com/HazelnutParadise/phego/issues/29)、[W-41](https://github.com/HazelnutParadise/phego/issues/48) | [#50](https://github.com/HazelnutParadise/phego/issues/50) | [#113](https://github.com/HazelnutParadise/phego/pull/113) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-53-implementation/docs/implementation/W-53.md) |
| W-54 | M0 | 全競品與工作負載登錄 | [W-01](https://github.com/HazelnutParadise/phego/issues/1)、[W-02](https://github.com/HazelnutParadise/phego/issues/2)、[W-03](https://github.com/HazelnutParadise/phego/issues/3) | [#7](https://github.com/HazelnutParadise/phego/issues/7) | [#114](https://github.com/HazelnutParadise/phego/pull/114) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-54-implementation/docs/implementation/W-54.md) |
| W-55 | M7 | 嚴格效能比較與統計驗收 | [W-54](https://github.com/HazelnutParadise/phego/issues/7)、[W-43](https://github.com/HazelnutParadise/phego/issues/49)、[W-56](https://github.com/HazelnutParadise/phego/issues/51)、[W-57](https://github.com/HazelnutParadise/phego/issues/52)、[W-58](https://github.com/HazelnutParadise/phego/issues/53) | [#57](https://github.com/HazelnutParadise/phego/issues/57) | [#115](https://github.com/HazelnutParadise/phego/pull/115) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-55-implementation/docs/implementation/W-55.md) |
| W-56 | M5 | WordPress 與 WooCommerce 真實驗收 | [W-34](https://github.com/HazelnutParadise/phego/issues/46)、[W-40](https://github.com/HazelnutParadise/phego/issues/47)、[W-41](https://github.com/HazelnutParadise/phego/issues/48)、[W-36](https://github.com/HazelnutParadise/phego/issues/45) | [#51](https://github.com/HazelnutParadise/phego/issues/51) | [#116](https://github.com/HazelnutParadise/phego/pull/116) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-56-implementation/docs/implementation/W-56.md) |
| W-57 | M5 | Laravel 與 BookStack 真實驗收 | [W-34](https://github.com/HazelnutParadise/phego/issues/46)、[W-40](https://github.com/HazelnutParadise/phego/issues/47)、[W-41](https://github.com/HazelnutParadise/phego/issues/48)、[W-36](https://github.com/HazelnutParadise/phego/issues/45) | [#52](https://github.com/HazelnutParadise/phego/issues/52) | [#117](https://github.com/HazelnutParadise/phego/pull/117) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-57-implementation/docs/implementation/W-57.md) |
| W-58 | M5 | 多生態與歷史專案驗收 | [W-50](https://github.com/HazelnutParadise/phego/issues/28)、[W-52](https://github.com/HazelnutParadise/phego/issues/43)、[W-34](https://github.com/HazelnutParadise/phego/issues/46)、[W-40](https://github.com/HazelnutParadise/phego/issues/47)、[W-36](https://github.com/HazelnutParadise/phego/issues/45) | [#53](https://github.com/HazelnutParadise/phego/issues/53) | [#118](https://github.com/HazelnutParadise/phego/pull/118) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-58-implementation/docs/implementation/W-58.md) |
| W-59 | M7 | 資料完整、故障、長時間與遷移驗收 | [W-53](https://github.com/HazelnutParadise/phego/issues/50)、[W-56](https://github.com/HazelnutParadise/phego/issues/51)、[W-57](https://github.com/HazelnutParadise/phego/issues/52)、[W-58](https://github.com/HazelnutParadise/phego/issues/53)、[W-41](https://github.com/HazelnutParadise/phego/issues/48) | [#58](https://github.com/HazelnutParadise/phego/issues/58) | [#119](https://github.com/HazelnutParadise/phego/pull/119) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-59-implementation/docs/implementation/W-59.md) |
| W-60 | M7 | 全目標認證與專案文件發布 | [W-49](https://github.com/HazelnutParadise/phego/issues/6)、[W-52](https://github.com/HazelnutParadise/phego/issues/43)、[W-54](https://github.com/HazelnutParadise/phego/issues/7)、[W-55](https://github.com/HazelnutParadise/phego/issues/57)、[W-59](https://github.com/HazelnutParadise/phego/issues/58)、[W-44](https://github.com/HazelnutParadise/phego/issues/59)、[W-45](https://github.com/HazelnutParadise/phego/issues/15) | [#60](https://github.com/HazelnutParadise/phego/issues/60) | [#120](https://github.com/HazelnutParadise/phego/pull/120) | [計畫](https://github.com/HazelnutParadise/phego/blob/codex/w-60-implementation/docs/implementation/W-60.md) |

## 文件驗證與交付限制

規劃驗證須核對60組一對一映射、全部需求與測試覆蓋、200個唯一驗收負責票、原始與補充依賴無循環，以及來源摘要不變。每個 draft 分支保存自己的計畫與驗證證據。若共用文件驗證紀錄發生衝突，依合併後的實際輸入重新產生。

執行 `python3 tools/validate_docs.py` 及 `python3 -m unittest discover -s tools -p 'test_*.py'`。文件驗證及工具測試只證明所檢查的文件與工具行為，不證明 PHP 引擎功能。每次合併前須核對該提交的實際 CI 結果。

這次維持既有產品方向、相容範圍、安全檢查、英文README標語與MIT授權。正式環境、軟體release、正式簽章金鑰與付費依賴沒有納入本輪操作。

## 減法審查

- Won't List：本輪不實作引擎，不建立正式發布或自動排程，因為交付是完整實作規劃及GitHub待辦。
- 建議移除：不建立空程式、假測試與空提交，計畫PR只放能審查的需求及實作安排。
- 建議簡化：沿用60個穩定工作包及既有規格，使用一份總表連到每張issue與draft，不另建平行的任務分類。
- 需要留意：60個draft保持獨立，不串成60層堆疊分支。後續實作按依賴更新一般提交，保留未完成與阻礙狀態。
