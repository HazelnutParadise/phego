<?php
class A
{
    public function __get($n)
    {
        return $this->other->$n;
    }
}
$a = new A;
echo "start\n";
$a->x->y = 1;
