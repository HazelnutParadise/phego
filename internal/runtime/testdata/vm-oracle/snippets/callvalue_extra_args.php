<?php
function One($a)
{
    echo $a, "\n";
    return $a % 0;
}
one(5, "extra", [1], null);
