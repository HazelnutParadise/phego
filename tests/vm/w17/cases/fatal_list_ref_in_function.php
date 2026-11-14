<?php
function f($s) {
    [&$a] = $s;
}
echo "before\n";
f("str");
