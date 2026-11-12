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

$s = "a";
$s++;
show($s);
$s = "z";
$s++;
show($s);
$s = "Az";
$s++;
show($s);
$s = "zz";
$s++;
show($s);
$s = "Zz";
$s++;
show($s);
$s = "a9";
$s++;
show($s);
$s = "A99";
$s++;
show($s);
$s = "5";
$s++;
show($s);
$s = "5.5";
$s++;
show($s);
$s = "1e2";
$s++;
show($s);
$s = " 5";
$s++;
show($s);
$s = "-3";
$s--;
show($s);
$s = "9223372036854775807";
$s++;
show($s);
