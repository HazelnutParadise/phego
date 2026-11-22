<?php
// CVE: CVE-2017-6441
// A rejected declare value is reported on the line of the first
// directive's name.
declare(
    ticks=1,
    unknown=BAR
);
echo "unreachable\n";
