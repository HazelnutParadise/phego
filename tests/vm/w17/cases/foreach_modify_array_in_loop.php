<?php
$a = [1, 2, 3];
foreach ($a as $v) {
    echo $v, " ";
    $a[] = $v * 10;
}
echo "\n";
echo $a[3], $a[4], $a[5], isset($a[6]) ? "more" : "end", "\n";
$b = [1, 2, 3];
foreach ($b as $k => $v) {
    $b[$k] = $v * 2;
    echo $b[$k], " ";
}
echo "\n";
echo $b[0], $b[1], $b[2], "\n";
$c = ["a" => 1, "b" => 2, "c" => 3];
foreach ($c as $k => $v) {
    unset($c["b"]);
    echo $k, $v, " ";
}
echo "\n";
echo isset($c["b"]) ? "b" : "no b", "\n";
$d = [1, 2, 3];
foreach ($d as $k => $v) {
    if ($k == 0) {
        $d = ["new"];
    }
    echo $v, " ";
}
echo "\n";
echo $d[0], isset($d[1]) ? "x" : "", "\n";
$e = [1, 2];
foreach ($e as $v) {
    unset($e);
    echo $v, " ";
}
echo "\n";
echo isset($e) ? "e set" : "e gone", "\n";
