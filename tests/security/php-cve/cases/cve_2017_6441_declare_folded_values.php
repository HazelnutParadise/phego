<?php
// CVE: CVE-2017-6441
// PHP folds a parenthesized literal, a heredoc or nowdoc without
// interpolation and a concatenation of literals into one ZVAL, so declare
// takes them like a literal. Every declare diagnostic is reported on the
// line of the first directive's name, not of the declare keyword.
declare(ticks=(1));
declare(ticks="1" . "2");
declare(ticks=<<<TICKS
5
TICKS);
declare(ticks=<<<'TICKS'
5
TICKS);
declare(ticks="-4294967295");
declare(ticks="1e999");
declare(
    ticks=1e30,
    ticks=2e30
);
declare(
    ticks=1,
    unknown=1
);
declare(ticks=(1e20));
echo "compiled\n";
