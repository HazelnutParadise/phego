<?php
class Point
{
    public function __construct(public $x, public $y)
    {
    }
}
$p = new Point(1, 2);
echo "point ", $p->x, $p->y, "\n";
$q = new Point(1);
