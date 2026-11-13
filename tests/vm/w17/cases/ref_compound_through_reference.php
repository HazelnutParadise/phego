<?php
$a = 1;
$b = &$a;
$b += 5;
$b .= "!";
echo $a, "\n";
$arr = ["k" => 1];
$r = &$arr["k"];
$arr["k"]++;
$r *= 10;
echo $arr["k"], " ", $r, "\n";
$s = "abc";
$t = &$s;
$t[1] = "X";
echo $s, "\n";
$u = null;
$w = &$u;
$w ??= "d";
echo $u, "\n";
