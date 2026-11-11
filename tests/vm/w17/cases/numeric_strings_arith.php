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

show("12" + 1);
show(" 12" + 1);
show("12 " + 1);
show(" 12 " + 1);
show("\n12\t" + 1);
show("1.5" + 1);
show("1e3" + 0);
show(".5" + 0);
show("-0" + 0);
show("+7" + 0);
show("0x1A" + 0);
show("012" + 0);
show("9223372036854775807" + 0);
show("9223372036854775808" + 0);
show("1" . "2" + 3);
show("10" / "4");
show("3" * "4");
show("2.5" - "0.5");
