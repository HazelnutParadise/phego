<?php
const DEFAULT_NAME = "anon", DEFAULT_SIZE = 4;
function greet($name = DEFAULT_NAME, $times = DEFAULT_SIZE - 2, $eol = PHP_EOL)
{
    $out = "";
    for ($i = 0; $i < $times; $i++) {
        $out .= "hi " . $name . $eol;
    }
    return $out;
}
echo greet();
echo greet("bob");
echo greet("ann", 1, "|"), "\n";
function later_default($x = LATER)
{
    return $x;
}
const LATER = [1, 2];
echo later_default()[1], later_default(5), "\n";
function predefined_default($m = M_PI, $i = PHP_INT_MAX)
{
    return $m > 3 && $i > 0 ? "ok" : "no";
}
echo predefined_default(), predefined_default(1, -1), "\n";
