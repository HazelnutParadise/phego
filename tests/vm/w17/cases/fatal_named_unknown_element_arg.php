<?php
function f($a = 1) {
    echo "called\n";
}
$arr = [];
f(a: $arr["missing ok"]);
f(nope: $arr["missing never read"]);
