<?php
// CVE: CVE-2007-1285
// CVE: CVE-2007-4670
// Destroying deeply nested arrays recursed once per level in the variable
// destructor and exhausted the C stack (MOPB-03-2007 and its variant).
$a = [];
for ($i = 0; $i < 100000; $i++) {
    $a = [$a];
}
$a = null;
echo "nested arrays released\n";
$b = [];
$cur = &$b;
for ($i = 0; $i < 100000; $i++) {
    $cur["k"] = [];
    $cur = &$cur["k"];
}
unset($cur);
$b = 0;
echo "nested references released\n";
