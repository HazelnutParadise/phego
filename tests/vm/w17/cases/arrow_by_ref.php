<?php
$x = 5;
$byRef = fn &($v) => $v;
echo "plain_call_return: ", $byRef($x), "\n";
echo "outer_after_plain_call: ", $x, "\n";

$const = fn &() => 1;
echo "const_by_ref_call: ", $const(), "\n";

$arr = [7, 8];
$first = fn &(array &$a) => $a[0];
echo "first_elem_plain: ", $first($arr), "\n";
echo "arr_after_first: ", $arr[0], "\n";

$assigned = $first($arr);
echo "assigned_plain_value: ", $assigned, "\n";

$twice = fn &($v) => $v * 2;
echo "expression_arg: ", $twice(4) + $twice(5), "\n";
echo "is_closure: ", $byRef instanceof Closure ? "yes" : "no", "\n";
