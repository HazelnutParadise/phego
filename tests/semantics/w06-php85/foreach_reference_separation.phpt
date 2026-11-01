--TEST--
Foreach by reference keeps its cursor across separation and tail deletion, and WeakMap self-cycles are collected
--FILE--
<?php
$arr = [1, 2, 3]; $seen = []; $done = false;
foreach ($arr as &$v) { $seen[] = $v; if (!$done) { $done = true; $c = $arr; $arr[2] = 30; } }
unset($v); echo implode(',', $seen), "\n";
$a = [1]; $seen = []; $once = false;
foreach ($a as &$v) { $seen[] = $v; if (!$once) { $once = true; unset($a[0]); $a[] = 2; } }
unset($v); echo implode(',', $seen), "\n";
class X { function __destruct() { echo "destruct X\n"; } }
$o = new X; $m = new WeakMap; $m[$o] = $o; $w = WeakReference::create($o); unset($o);
echo "collected ", gc_collect_cycles(), "\n";
echo count($m), " ", var_export($w->get() === null, true), "\n";
--EXPECT--
1,2,30
1,2
collected destruct X
1
0 true
