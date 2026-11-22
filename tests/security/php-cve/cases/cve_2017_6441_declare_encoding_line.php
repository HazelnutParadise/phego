<?php
// CVE: CVE-2017-6441
// The declare(encoding=...) warning comes from the parser at the closing
// parenthesis, so every one of them precedes the compile-time warnings,
// such as the one for the first directive below.
declare(
    encoding="UTF-8",
    unknown=1
);
declare(other=1);
declare(
    encoding="UTF-8"
);
echo "compiled\n";
