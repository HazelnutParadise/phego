<?php
class A
{
    public $v = 1;
}
$o = new A;
echo "value ", $o->v + 1, "\n";
echo $o + 1, "\n";
