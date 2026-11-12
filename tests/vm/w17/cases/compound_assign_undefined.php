<?php
function show($v)
{
    if ($v === null) {
        echo "NULL\n";
    } elseif ($v === true) {
        echo "bool(true)\n";
    } elseif ($v === false) {
        echo "bool(false)\n";
    } elseif ($v !== $v) {
        echo "float(NAN)\n";
    } elseif ((array)$v === $v) {
        echo "array\n";
    } elseif ((float)$v === $v) {
        echo "float(", $v, ")\n";
    } elseif ((string)$v === $v) {
        echo "string(\"", $v, "\")\n";
    } else {
        echo "int(", $v, ")\n";
    }
}

$a += 1;
show($a);
$b .= "x";
show($b);
$c -= 2.5;
show($c);
$d *= 3;
show($d);
$e |= 4;
show($e);
$arr = [];
$arr["k"] += 5;
show($arr["k"]);
$arr[3] .= "s";
show($arr[3]);
$p **= 2;
show($p);
