<?php
// CVE: CVE-2007-1383
// PHP 4 kept a 16-bit reference counter, so 65536 references wrapped it to
// zero and the shared value was destroyed twice.
$v = "shared";
$refs = [];
for ($i = 0; $i < 70000; $i++) {
    $refs[] = &$v;
}
$refs[69999] = "changed through the last reference";
echo $v, "\n";
$refs = null;
echo $v, "\n";
$copies = [];
for ($i = 0; $i < 70000; $i++) {
    $copies[] = $v;
}
$copies = null;
echo $v, "\n";
$o = new stdClass;
$o->n = 1;
$holders = [];
for ($i = 0; $i < 70000; $i++) {
    $holders[] = $o;
}
$holders = null;
echo $o->n, "\n";
