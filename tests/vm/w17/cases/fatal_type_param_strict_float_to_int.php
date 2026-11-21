<?php
declare(strict_types=1);
class Counter
{
    public function add(int $n)
    {
        return $n;
    }
}
$c = new Counter;
echo "add ", $c->add(1), "\n";
$c->add(1.0);
