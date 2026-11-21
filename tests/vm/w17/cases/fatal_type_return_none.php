<?php
function maybe($x): int
{
    if ($x) {
        return 1;
    }
}
echo maybe(true), "\n";
echo maybe(false), "\n";
