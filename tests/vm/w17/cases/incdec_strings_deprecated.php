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

$s = "";
$s++;
show($s);
$s = "a-";
$s++;
show($s);
$s = "-";
$s++;
show($s);
$s = "5 ";
$s++;
show($s);
$s = "abc";
$s--;
show($s);
$s = "";
$s--;
show($s);
$s = "z ";
$s++;
show($s);
