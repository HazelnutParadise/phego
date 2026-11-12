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

show(12 & 10);
show(12 | 10);
show(12 ^ 10);
show(~0);
show(~5);
show(~-1);
show(-1 & 0xFF);
show(~1.9);
show("12" & 10);
show("ab" & "  ");
show("ab" | "  ");
show("AB" ^ "  ");
show("abc" & "a");
show(~"AB" === "\xBE\xBD");
show(true | 2);
show(null ^ 7);
show(1.5 | 0);
show("5" | "3");
$x = 6;
$x &= 3;
show($x);
$x |= 8;
show($x);
$x ^= 15;
show($x);
