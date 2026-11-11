<?php
function g($a, $b, $c)
{
    $a = "changed";
    unset($b);
    return 1 % 0;
}
g(1.0, "a\nlong string with more than fifteen", [1], null, true, 1e100, -0.0, "x\\y\0z", 7, "exactly15chars!", "sixteen chars!!!", 1e400, -1e400, 2.5, 100.0, 0.1, "\t\r\f\v\e\x7f\x80");
