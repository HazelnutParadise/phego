<?php
foreach ([1, 2, 3] as &$v) {
    $v++;
    echo $v;
}
echo "\n";
function make() { return [4, 5]; }
foreach (make() as $k => &$v) {
    echo $k, $v;
}
echo "\n";
