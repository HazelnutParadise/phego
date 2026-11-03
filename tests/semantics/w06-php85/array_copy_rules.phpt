--TEST--
Array copies dereference sole references and keep PHP key order and next-free rules
--FILE--
<?php
$a = [1, 2]; $r = &$a[0]; unset($r);
$b = $a; $b[0] = 9;
echo $a[0], ":", $b[0], "\n";
$x = 1; $c = [&$x]; $d = $c; $d[0] = 5; echo $x, ":", $c[0], "\n";
$n = [-5 => 'a']; $n[] = 'b'; var_dump(array_keys($n));
$p = [3 => 'x']; unset($p[3]); $p[] = 'y'; var_dump(array_keys($p));
$m = [PHP_INT_MAX => 1]; try { $m[] = 2; } catch (Error $e) { echo get_class($e), ": ", $e->getMessage(), "\n"; }
$o = ['b' => 1, 'a' => 2, 10 => 3, '7' => 4]; foreach ($o as $k => $v) { echo var_export($k, true), "=>$v "; } echo "\n";
$arr = [1,2,3]; foreach ($arr as $k => &$v) { if ($k == 0) { $arr[] = 4; } $v *= 10; } unset($v); echo implode(',', $arr), "\n";
--EXPECT--
1:9
5:5
array(2) {
  [0]=>
  int(-5)
  [1]=>
  int(-4)
}
array(1) {
  [0]=>
  int(4)
}
Error: Cannot add element to the array as the next element is already occupied
'b'=>1 'a'=>2 10=>3 7=>4 
10,20,30,40
