<?php
// fatal_throw_uncaught_trace_args: uncaught exception from four nested calls with long string, array, object, closure, null, bool, float and variadic arguments
function level4($text, $list, $obj, $fn, $nothing, $flag, $ratio, ...$rest)
{
    throw new RuntimeException("deep");
}

function level3($text, $list, $obj, $fn)
{
    return level4("a fairly long string argument", $list, $obj, $fn, null, true, 2.5, 7, "two");
}

function level2($list, $obj)
{
    return level3("x", $list, $obj, function ($v) {
        return $v;
    });
}

function level1()
{
    return level2(["k" => 1, "j" => [2]], new stdClass());
}

echo "start\n";
level1();
echo "never\n";
