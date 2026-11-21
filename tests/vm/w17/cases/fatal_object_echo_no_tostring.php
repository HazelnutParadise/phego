<?php
class Plain
{
    public $v = "inner";
}
$p = new Plain;
echo "inner value: ", $p->v, "\n";
echo $p, "\n";
