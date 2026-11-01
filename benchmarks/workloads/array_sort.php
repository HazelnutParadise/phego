<?php
// Indexed array fill, sort and ordered checksum.
$x = 1; $a = array();
for ($i = 0; $i < 200000; $i++) { $x = ($x * 75 + 74) % 65537; $a[] = $x; }
sort($a);
$sum = 0;
for ($i = 0; $i < 200000; $i += 997) { $sum = ($sum * 31 + $a[$i]) % 1000003; }
echo count($a), " ", $a[0], " ", $a[199999], " ", $sum, "\n";
