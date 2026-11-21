<?php
class Flag
{
    public bool $on = false;
}
$f = new Flag;
$f->on = 1;
echo "bool from int: ", ($f->on ? "true" : "false"), "\n";
$f->on = [];
