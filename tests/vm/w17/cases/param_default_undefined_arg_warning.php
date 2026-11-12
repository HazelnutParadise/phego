<?php
function f($a = "dflt", $b = "dflt2")
{
    echo $a === null ? "null" : $a, $b === null ? "null" : $b, "\n";
}
f($missing);
f("ok", $missing2);
f($missing3, $missing4);
$arr = [];
f($arr["k"]);
f($arr["k"]["j"], "after");
