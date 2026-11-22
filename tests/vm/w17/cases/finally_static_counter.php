<?php
// finally_static_counter: a static variable incremented in finally counts every call, including early returns and caught exceptions
class Bad extends Exception {}

function counted($mode)
{
    static $calls = 0;
    try {
        if ($mode === 'early') {
            return "early";
        }
        if ($mode === 'throw') {
            throw new Bad("bad");
        }
        return "normal";
    } catch (Bad $e) {
        return "caught";
    } finally {
        $calls = $calls + 1;
        echo "finally count: ", $calls, " mode: ", $mode, "\n";
    }
}

$r = counted('early');
echo "r1: ", $r, "\n";
$r = counted('normal');
echo "r2: ", $r, "\n";
$r = counted('throw');
echo "r3: ", $r, "\n";
$r = counted('early');
echo "r4: ", $r, "\n";
