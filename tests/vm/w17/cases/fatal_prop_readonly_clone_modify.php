<?php
class Box
{
    public function __construct(public readonly int $v)
    {
    }
}
$b = new Box(1);
$c = clone $b;
echo "clone has ", $c->v, "\n";
$c->v = 2;
