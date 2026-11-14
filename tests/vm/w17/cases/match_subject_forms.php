<?php
$arr = [1 => "one"];
echo match ($arr[1]) { "one" => "O" }, "\n";
echo match ($arr[2]) { null => "N" }, "\n";
$s = "xyz";
echo match ($s[1]) { "y" => "Y" }, "\n";
echo match ($s[9]) { "" => "empty" }, "\n";
$k = 0;
echo match ($k++) { 0 => "first", 1 => "second" }, $k, "\n";
echo match ($k++) { 0 => "first", 1 => "second" }, $k, "\n";
echo match ("a" . "b") { "ab" => "concat" }, "\n";
echo match (1 + 1) { 2 => "sum" }, "\n";
function side()
{
    echo "side!";
    return 5;
}
echo match (side()) { 5 => "five" }, "\n";
$v = 3;
echo match ($v) { $v => "self" }, "\n";
echo match ($v) { $v + 1 => "no", $v => "yes" }, "\n";
