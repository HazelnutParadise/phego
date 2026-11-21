<?php
class Box
{
    public function __construct(public readonly int $v)
    {
    }
}
$b = new Box(1);
echo "taking reference\n";
$r = &$b->v;
