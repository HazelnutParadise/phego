<?php
$v = 1;
$arr = [];
$arr["a"] = &$v;
$arr["b"]["c"] = &$v;
$arr[] = &$v;
$v = 2;
echo $arr["a"], $arr["b"]["c"], $arr[0], "\n";
$arr["a"] = 3;
echo $v, "\n";
$src = ["p" => ["q" => 1]];
$dst = [];
$dst["x"] = &$src["p"]["q"];
$dst["x"] = 8;
echo $src["p"]["q"], "\n";
$src["p"]["q"] = 9;
echo $dst["x"], "\n";
$src["p"] = 0;
echo $dst["x"], "\n";
