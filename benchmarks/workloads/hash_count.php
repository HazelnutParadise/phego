<?php
// String-keyed hash table updates and ordered iteration.
$x = 7; $counts = array();
for ($i = 0; $i < 300000; $i++) {
    $x = ($x * 75 + 74) % 65537;
    $key = 'k' . ($x % 5000);
    if (isset($counts[$key])) { $counts[$key]++; } else { $counts[$key] = 1; }
}
ksort($counts);
$sum = 0; $n = 0;
foreach ($counts as $k => $v) { $sum = ($sum * 33 + $v + strlen($k)) % 1000003; $n++; }
echo $n, " ", $sum, "\n";
