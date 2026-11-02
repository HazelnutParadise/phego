--TEST--
Foreach by reference restarts on a replaced array
--FILE--
<?php
$arr = [1, 2, 3]; $seen = [];
foreach ($arr as &$v) { $seen[] = $v; if ($v === 1) { $arr = [4, 5]; } }
unset($v); echo implode(',', $seen), "\n";
$arr = [1, 2, 3]; $seen = [];
foreach ($arr as &$v) { $seen[] = $v; if ($v === 2) { $arr = [7, 8, 9, 10]; } }
unset($v); echo implode(',', $seen), "\n";
--EXPECT--
1,4,5
1,2,7,8,9,10
