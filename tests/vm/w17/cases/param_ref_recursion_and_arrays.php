<?php
function fill(&$a, $n)
{
    if ($n == 0) {
        return;
    }
    $a[] = $n;
    fill($a, $n - 1);
}
$list = [];
fill($list, 3);
echo $list[0], $list[1], $list[2], "\n";
function bump(&$arr)
{
    foreach ($arr as &$v) {
        $v = $v . "!";
    }
}
$words = ["a", "b"];
$copy = $words;
bump($words);
echo $words[0], $words[1], $copy[0], $copy[1], "\n";
function grow(&$x) { $x[] = 1; }
$s = null;
grow($s);
echo $s[0], "\n";
