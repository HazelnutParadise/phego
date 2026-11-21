<?php
class Num
{
    public int $n = 1;
}
function set_text(&$x)
{
    $x = "text";
}
$o = new Num;
echo "start ", $o->n, "\n";
set_text($o->n);
