<?php
class Bag
{
    public array $items = [];
}
$b = new Bag;
echo "assigning\n";
$b->items = "not an array";
