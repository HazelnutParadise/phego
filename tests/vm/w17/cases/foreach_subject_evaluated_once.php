<?php
function tick()
{
    echo "T";
    return [1, 2, 3];
}
foreach (tick() as $v) {
    echo $v;
}
echo "\n";
$i = 0;
foreach ([$i++, $i++, $i++] as $k => $v) {
    echo $k, $v, " ";
}
echo $i, "\n";
$n = 0;
foreach ([1, 2, 3] as $v) {
    $n += $v;
    foreach ([tick()] as $w) {
        echo count_it($w);
    }
}
echo "\n", $n, "\n";
function count_it($arr)
{
    $c = 0;
    foreach ($arr as $x) {
        $c++;
    }
    return $c;
}
