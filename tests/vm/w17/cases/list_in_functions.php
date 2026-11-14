<?php
function pair() { return [1, 2]; }
function swap($p) { [$p[0], $p[1]] = [$p[1], $p[0]]; return $p; }
function sum($row) { [$a, $b] = $row; return $a + $b; }
function first(&$arr) { [&$f] = $arr; $f = 'first'; }
function depth($n) {
    if ($n === 0) { return 0; }
    [$d] = [depth($n - 1) + 1];
    return $d;
}
[$a, $b] = pair();
echo $a, $b, "\n";
$s = swap([1, 2]);
echo $s[0], $s[1], "\n";
echo sum([3, 4]), "\n";
$arr = [1, 2];
first($arr);
echo $arr[0], "\n";
echo depth(5), "\n";
function missing($r) { [$x, $y] = $r; return $y; }
echo missing([1]) === null ? "null" : "?", "\n";
function keyed($r) { ['k' => $k] = $r; return $k; }
echo keyed(['k' => 'K']), "\n";
echo keyed(5) === null ? "null" : "?", "\n";
