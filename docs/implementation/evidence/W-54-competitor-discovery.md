# W-54｜PHP 競品來源線索

本紀錄是 W-54 提前進行的唯讀來源發現，對應 REQ-074、REQ-088、REQ-089、REQ-090 及 T-147、T-148、T-175 至 T-180。它不是完整競品清冊，也不是建置、相容性、效能或安全驗收；上述情境仍為 `not_run`。

2026-10-29 核對[來源清單](W-54-candidate-sources.json)中的 31 份上游原始檔，涵蓋 13 個儲存庫。每筆記錄固定提交、原始網址、檔案大小與 SHA-256。下列內容是對該提交上游文件的觀察，提交不等於最新正式發行版本。表中所述能力均為上游聲明，尚未自行建置或執行。

| 候選 | 上游描述與待確認事項 |
|---|---|
| [HippyVM](https://github.com/hippyvm/hippyvm) | 以 RPython/PyPy 實作 PHP 的 VM；README 提供執行方式，並列出 64-bit x86 Linux 建置測試環境。精確 PHP 版本、可建置性與擴充相容性待驗。 |
| [JPHP](https://github.com/jphp-group/jphp) | 將 PHP 編譯為 JVM bytecode，宣稱涵蓋許多 PHP 7.1 以上特性。上游明說不打算實作 Zend runtime libraries，字串採 UTF-16；不能假定標準函式或字串語意相同。 |
| [phc](https://github.com/pbiggar/phc) | 歷史 PHP compiler，上游標示未維護、語法框架停留在 PHP 5.2 且缺少物件支援。其 configure 檔要求 PHP embed SAPI/libphp5，parser 授權記錄也指出衍生自 Zend；不能視為無 Zend 依賴的獨立 runtime。 |
| [ePHP](https://github.com/bragful/ephp) | Erlang PHP interpreter，上游明說尚未完成。COPYING 與套件設定標示 LGPL 2.1，README 另有 LGPL 1.2 字樣，授權說明不一致，需釐清。 |
| [GraalPHP](https://github.com/abertschi/graalphp) | 實驗性 GraalVM PHP 7.4+ JIT/compiler runtime；上游說明實作範圍只涵蓋用於特定合成測試的 PHP 子集合。不能據此推論網站或完整應用可執行。 |
| [Phalanger](https://github.com/DEVSENSE/Phalanger) | 上游描述為 PHP 5.4 的 .NET/Mono runtime/compiler，並稱其為 PeachPie 前身。兩者仍須分開鎖定版本及工作負載。 |
| [Roadsend PHP](https://github.com/weyrick/roadsend-php) | README 描述原始實作可將 PHP 編譯為原生執行檔。精確 PHP 版本、授權、建置依賴與可執行性尚待核對。 |
| [Roadsend Raven](https://github.com/weyrick/roadsend-php-raven) | 上游描述為 Roadsend 的 C++/LLVM 重寫，並明說目前停用程式碼產生。這是歷史未完成候選，不能當成已可執行的編譯器；須找到可執行版本才能比較。 |
| [phprs](https://github.com/yingkitw/phprs) | 上游描述為 Rust PHP runtime，含 lexer、parser 與自己的 opcode/VM。Cargo.toml 記載 `0.1.19`，不代表已驗證的 PHP 相容範圍。 |
| [php-rust / phpr](https://github.com/francescotinti/php-rust) | 上游描述為自行實作的 PHP 8.5 Rust bytecode VM；與 phprs 是不同專案。上游對應用與擴充的聲明仍需實際驗證。 |
| [phpscript](https://github.com/titpetric/phpscript) | Go PHP interpreter，具有 bytecode backend 與 HTTP server；上游明示只支援語言與標準函式的子集合。共同工作負載仍需確認。 |
| [Elephc](https://github.com/illegalstudio/elephc) | 上游描述將靜態 PHP 子集合編譯成原生程式，並提供處理動態 eval 的可選 bridge；Cargo.toml 記載 `0.27.6`。比較時須區分一般 PHP、專案自有擴充與 bridge 設定，不可修改應用以製造等價結果。 |
| [Quercus development copy](https://github.com/CleverCloud/Quercus) | README 稱其為 Caucho 的 Java PHP 實作之開發副本；原始檔包含 GPLv2 授權文本及商業方案說明。它不能直接代表其他 fork 或產品版本，精確 PHP profile 與可取得版本待確認。 |

純 Hack、僅有 PHP 語法分析的工具，以及只封裝其他 PHP 引擎的部署工具，不能直接歸類為獨立 PHP 執行引擎；仍須另行辨識適用的部署組合。各候選的正式 release、完整授權、工具鏈、平台、設定、安全模式及共同工作負載尚未逐一鎖定。

沒有建置、執行或量測這些候選，也沒有核對真實應用與官方語意。無法建置、不支援或資料不足都保持未知，不視為 Phego 的效能勝利。W-54 仍待完成涵蓋範圍審查、比較矩陣與可重現執行證據。
