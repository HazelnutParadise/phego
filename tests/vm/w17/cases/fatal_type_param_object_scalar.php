<?php
function inspect(object $o)
{
    return "object";
}
echo inspect(new class_x), "\n";
class class_x
{
}
echo inspect("class_x"), "\n";
