--TEST--
Shared acyclic arrays compare normally and foreach keeps its place after deleting the current element
--FILE--
<?php
$x = [1]; $a = [$x]; $b = [$a]; var_dump($a === $b);
$arr = [1, 2, 3]; $seen = []; $once = false;
foreach ($arr as &$v) { $seen[] = $v; if ($v === 2 && !$once) { $once = true; unset($arr[1]); $copy = $arr; } }
unset($v); echo implode(',', $seen), "\n";
--EXPECT--
bool(false)
1,2,3
