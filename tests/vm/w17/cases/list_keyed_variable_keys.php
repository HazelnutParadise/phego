<?php
[$undefKey => $a] = ['' => 'empty'];
echo $a, "\n";
$k = null;
[$k => $b] = ['' => 'nullkey'];
echo $b, "\n";
$f = 2.5;
[$f => $c] = [2 => 'two'];
echo $c, "\n";
$t = true;
[$t => $d] = [1 => 'one'];
echo $d, "\n";
$arr = [1];
[$undefKey2 => $e] = $arr;
echo $e === null ? "null" : "?", "\n";
$s = 'str';
[$s => $g] = 5;
echo $g === null ? "null" : "?", "\n";
