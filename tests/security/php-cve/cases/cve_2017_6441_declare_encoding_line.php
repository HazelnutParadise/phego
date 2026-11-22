<?php
// CVE: CVE-2017-6441
// The declare(encoding=...) warning comes from the parser at the closing
// parenthesis, before the compile-time warning of the first directive.
declare(
    encoding="UTF-8",
    unknown=1
);
echo "compiled\n";
