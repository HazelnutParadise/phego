<?php
class Money
{
    public function __construct(public readonly int $amount)
    {
    }
}
$m = new Money(5);
echo "amount ", $m->amount, "\n";
$m->amount = 6;
