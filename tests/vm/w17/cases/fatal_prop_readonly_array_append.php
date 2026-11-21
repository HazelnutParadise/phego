<?php
class Bag
{
    public function __construct(public readonly array $items = [])
    {
    }
}
$b = new Bag([1]);
echo "first ", $b->items[0], "\n";
$b->items[] = 2;
