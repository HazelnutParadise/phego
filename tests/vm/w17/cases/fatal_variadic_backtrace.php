<?php
function fails($a, $b = "dflt", ...$rest) {
    $rest[] = "added";
    $a = "reassigned";
    return undefined_function_here();
}
function middle(...$args) {
    return fails(...$args);
}
function top(&...$refs) {
    return middle(1, "two", 3.5, null, true, [1, 2], "a string that is longer than fifteen bytes", ...$refs);
}
$x = "x";
$y = ["y"];
echo top($x, $y);
