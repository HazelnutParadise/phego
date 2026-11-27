<?php
// catch_engine_trace: an engine TypeError raised three calls deep, with its trace, line, file and string form
function level3(int $n)
{
    return $n;
}

function level2($s)
{
    return level3($s);
}

function level1($s)
{
    return level2($s);
}

try {
    $r = level1("not a number");
} catch (TypeError $e) {
    echo "class: ", $e::class, "\n";
    echo "message: ", $e->getMessage(), "\n";
    echo "line: ", $e->getLine(), "\n";
    echo "file: ", $e->getFile(), "\n";
    echo "trace:\n", $e->getTraceAsString(), "\n";
    $text = (string) $e;
    echo "string_starts: ", substr_test($text), "\n";
}

function substr_test($text)
{
    return $text === "" ? "empty" : "present";
}

try {
    $r = [] + 1;
} catch (TypeError $e) {
    echo "direct_line: ", $e->getLine(), "\n";
    echo "direct_trace_frames: ", $e->getTraceAsString(), "\n";
}
echo "end: ok\n";
