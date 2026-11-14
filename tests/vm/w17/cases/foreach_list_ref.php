<?php
$rows = [[1, 2], [3, 4]];
foreach ($rows as [&$p, $q]) {
    $p *= 10;
}
unset($p);
echo $rows[0][0], $rows[0][1], $rows[1][0], $rows[1][1], "\n";
$recs = [['v' => 1], ['v' => 2]];
foreach ($recs as $k => ['v' => &$v]) {
    $v = $k . ':' . $v;
}
unset($v);
echo $recs[0]['v'], " ", $recs[1]['v'], "\n";
$m = [[[1]], [[2]]];
foreach ($m as [[&$c]]) {
    $c = -$c;
}
unset($c);
echo $m[0][0][0], $m[1][0][0], "\n";
$short = [[1]];
foreach ($short as [&$x, &$y]) {
    $y = 'Y';
}
unset($x, $y);
echo $short[0][0], $short[0][1], "\n";
$out = [];
$src = [[5, 6]];
foreach ($src as [&$out[0], $out[1]]) {
}
$out[0] = 50;
echo $src[0][0], $src[0][1], "\n";
foreach ([[1]] as [&$t]) {
    $t = 2;
    echo $t, "\n";
}
unset($t);
$rows2 = [[1], [2]];
foreach ($rows2 as [&$r]) {
    $r++;
}
echo $rows2[0][0], $rows2[1][0], $r, "\n";
$r = 'after';
echo $rows2[1][0], "\n";
