# Rules for `internal/frontend`

Each line is a condition and what to do. The story behind a line is in the commit that added it (`git log -S '<words from the line>'`).

- Checking lexer or parser behaviour against PHP 8.5.12: use `token_get_all($src)` for tokens and `token_get_all($src, TOKEN_PARSE)` for accept/reject and the error line, over the corpus that `tools/frontend_corpus.py` extracts from php-src (kept outside the repository). Encode dumps with `JSON_INVALID_UTF8_SUBSTITUTE`, or files in other encodings silently drop records. PHP reports a syntax error on the line where the unexpected token ends.
- Before asking for another review of a grammar change: run the token-mutation differential (`tests/frontend/mutate.php`, compared through `PHEGO_LEXER_CORPUS=<dir>`). The php-src corpus is almost all valid code, so reviews found one rejected-input edge case per round until mutants covered them. Mutant file names are 16 hex digits plus `.php`, and scripts are written before they are mounted, because Docker creates a directory for a missing host file.
