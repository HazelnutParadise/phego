<?php
declare(strict_types=1);
function flag(bool $b)
{
    return $b ? "on" : "off";
}
echo flag(true), "\n";
echo flag(1), "\n";
