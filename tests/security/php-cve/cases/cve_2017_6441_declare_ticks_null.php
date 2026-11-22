<?php
// CVE: CVE-2017-6441
// declare(ticks=null): a constant name is not a literal, so PHP 8.5 rejects
// it at compile time instead of converting a missing value.
declare(ticks=null);
echo "unreachable\n";
