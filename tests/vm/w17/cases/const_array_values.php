<?php
const TABLE = [1, "k" => ["z", "w"], 7 => "s"];
echo TABLE["k"][0], TABLE[0], TABLE[7], "\n";
echo TABLE["q"] ?? "default", "\n";
echo TABLE["q"];
echo "\n";
echo TABLE["k"][5];
echo "\n";
const SCALAR = 5;
echo SCALAR[0];
echo "\n";
$copy = TABLE;
$copy["k"][0] = "changed";
echo TABLE["k"][0], $copy["k"][0], "\n";
foreach (TABLE as $k => $v) {
    echo $k, "=", $v === TABLE["k"] ? "array" : $v, ";";
}
echo "\n";
echo isset(TABLE["k"]), isset(TABLE["none"]) ? "set" : "unset", empty(TABLE["k"][1]) ? "e" : "ne", "\n";
function take($a)
{
    return $a[7];
}
echo take(TABLE), "\n";
const EMPTY_LIST = [];
echo empty(EMPTY_LIST) ? "empty" : "full", "\n";
echo EMPTY_LIST[0] ?? "none", "\n";
