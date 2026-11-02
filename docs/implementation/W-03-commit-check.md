# W-03 執行紀錄的提交檢查

`python3 tools/check_evidence_commit.py --results records.json` 先執行既有紀錄及證據摘要檢查，再確認紀錄中的 40 字元提交編號是此 Git 工作樹中存在的 commit 物件。工作樹須為儲存庫根目錄。缺少物件、非 commit、淺層複本缺少提交或輸入無效時，命令以狀態碼 2 失敗。未執行任何項目時不主張提交已驗證。

輸出保留完整測試分母，並標示 `execution_verification: not_verified` 及 `product_acceptance: not_claimed`。提交存在不證明命令曾在該提交執行，也不證明測試結果、來源鎖或平台設定正確。此工具是 REQ-076、REQ-079 的局部檢查，不代表 T-151、T-152、T-157 或 T-158 已通過。

`python3 -m unittest discover -s tools -p 'test_evidence_commit.py'` 使用暫存 Git 儲存庫驗證正常提交、缺少物件、非 commit 物件、空紀錄、錯誤根目錄及 Git 環境變數。實際產品驗收與來源身分驗證尚未完成。
