<?php
$x = 1;
echo match (true) { $x > 0 => "pos", default => "neg" }, "\n";
echo match ($x) { 1 => "one" . "x", }, "\n";
$f = match ($x) { 1 => 2.5, 2 => "s" };
echo $f, "\n";
echo match ("1") { 1 => "int", "1" => "str" }, "\n";
echo match (1.0) { 1 => "int", default => "other" }, "\n";
$a = [1];
echo match ($a) { [1] => "arr", default => "no" }, "\n";
echo match (null) { default => "d" }, "\n";
function m($v)
{
    return match ($v) { 1, 2, 3 => "low", 4 => "four", default => "hi" };
}
echo m(2), m(4), m(9), m("1"), "\n";
$k = 0;
echo match ($k++) { 0 => "a", 1 => "b" }, $k, "\n";
echo match (3) { 1, 2 => "a", 3 => "b", default => "c" }, "\n";
echo match (true) { 1 > 2 => "x", 3 > 2 => "y" }, "\n";
echo match ("b") { "a" => 1, "b" => 2, "c" => 3 }, "\n";
echo match (PHP_INT_SIZE) { 4 => "32-bit", 8 => "64-bit" }, "\n";
