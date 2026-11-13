<?php
$a = 3;
$r = ($b = &$a);
$b = 4;
echo $r, " ", $a, "\n";
echo ($c = &$a) + 1, "\n";
$arr = [1, 2];
echo ($arr[0] = &$a) * 2, "\n";
echo $arr[0], "\n";
