<?php
$a = [];
$a["1"] = "a";
$a["01"] = "b";
$a[true] = "c";
$a[2.7] = "d";
$a[null] = "e";
$a[] = "f";
$a[-3] = "g";
$a[] = "h";
foreach ($a as $k => $v) {
    echo $k, "=>", $v, ";";
}
echo "\n";
foreach ($a as $k => $v) {
    echo $k === 1 ? "int1" : ($k === "01" ? "str01" : ($k === "" ? "empty" : "other")), " ";
}
echo "\n";
