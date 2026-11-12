<?php
function first()
{
    echo $u1, "|";
    return second();
}

function second()
{
    $a = [];

    return $a["missing"];
}
echo first(), "\n";
echo second(), "\n";
