<?php
echo "never printed\n";
function f($a = 1, $a = 2)
{
    return $a;
}
