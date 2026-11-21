<?php
class Box
{
    public function __construct(public readonly int $v)
    {
    }
}
$b = new Box(1);
echo "unsetting\n";
unset($b->v);
