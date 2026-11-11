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

$a = ($b = 5) + 1;
show($a);
show($b);
$c = $d = $e = "chain";
show($c . $d . $e);
$x = [1, 2];
$y = $x;
$y[0] = 99;
show($x[0]);
show($y[0]);
$s = "abc";
$t = $s;
$t .= "d";
show($s);
show($t);
show($z = 3.5);
$n = null;
show($n);
