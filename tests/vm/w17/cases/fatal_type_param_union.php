<?php
function id(int|string $v)
{
    return $v;
}
echo id(1), id("a"), "\n";
echo id([]), "\n";
