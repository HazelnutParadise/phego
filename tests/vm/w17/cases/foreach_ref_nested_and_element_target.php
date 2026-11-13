<?php
$m = [[1, 2], [3, 4]];
foreach ($m as &$row) {
    foreach ($row as &$cell) {
        $cell += 10;
    }
    unset($cell);
}
unset($row);
echo $m[0][0], $m[0][1], $m[1][0], $m[1][1], "\n";
$data = ["a" => [1, 2]];
foreach ($data["a"] as &$v) {
    $v = -$v;
}
unset($v);
echo $data["a"][0], $data["a"][1], "\n";
$out = [];
$src = [5, 6];
foreach ($src as $k => &$out[$k]) {
}
$out[0] = 50;
echo $src[0], " ", $src[1], "\n";
