<?php
class Money
{
    public function __construct(public readonly int $amount)
    {
    }

    public function double()
    {
        $this->amount *= 2;
    }
}
$m = new Money(5);
echo "doubling\n";
$m->double();
