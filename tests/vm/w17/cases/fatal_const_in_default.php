<?php
function f($x = PHP_EOL, $y = UNDEF)
{
    return $x;
}
echo f("a"), "\n";
echo f();
